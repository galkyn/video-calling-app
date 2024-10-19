package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for simplicity
	},
}

type User struct {
	Connection *websocket.Conn
	Peer       *webrtc.PeerConnection
}

var users = make(map[string]*User)
var usersMutex sync.Mutex

type Message struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	From      string      `json:"from"`
	To        string      `json:"to"`
	Candidate interface{} `json:"candidate,omitempty"`
	Offer     interface{} `json:"offer,omitempty"`
	Answer    interface{} `json:"answer,omitempty"`
}

var animalNames = []string{
	"🐶 Puppy", "🐱 Kitty", "🐭 Mouse", "🐹 Hamster", "🐰 Bunny", "🦊 Fox", "🐻 Bear", "🐼 Panda",
	" Koala", "🐯 Tiger", "🦁 Lion", "🐮 Cow", "🐷 Piggy", "🐸 Froggy", "🐵 Monkey", "🐔 Chicken",
	"🦄 Unicorn", "🐙 Octopus", "🦋 Butterfly", "🦜 Parrot", "🦒 Giraffe", "🦘 Kangaroo", "🦥 Sloth", "🦦 Otter",
}

var client *mongo.Client
var callsCollection *mongo.Collection

func init() {
	rand.Seed(time.Now().UnixNano())

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://videoCallUser:password@localhost:27017/videoCallDB"
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}

	// Check the connection
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	callsCollection = client.Database("videoCallDB").Collection("calls")
	log.Println("Connected to MongoDB")
}

func main() {
	// Load SSL certificates
	cert, err := tls.LoadX509KeyPair("/app/certs/certificate.crt", "/app/certs/private.key")
	if err != nil {
		log.Fatalf("Failed to load certificates: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleConnections)

	server := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	log.Println("Go server started at https://0.0.0.0:8080")
	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.Fatalf("Failed to start HTTPS server: %v", err)
	}

	// Start database stats logging in a separate goroutine
	go logDatabaseStats()

	// Add this function to log all calls (for debugging purposes)
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			logAllCalls()
		}
	}()
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	log.Println("Attempting WebSocket connection")

	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

	// Handle OPTIONS preflight requests
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Upgrade the HTTP request to WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to WebSocket: %v", err)
		return
	}
	defer ws.Close()

	log.Println("WebSocket connection established")

	// Generate new client ID
	clientID := generateClientID()
	usersMutex.Lock()

	// If a user with the same ID exists, disconnect the old one
	if existingUser, exists := users[clientID]; exists {
		log.Printf("Disconnecting old client with ID: %s", clientID)
		existingUser.Connection.Close() // Disconnect old connection
		delete(users, clientID)         // Remove old clientID
	}
	users[clientID] = &User{Connection: ws}
	usersMutex.Unlock()

	// Send client ID to the client
	log.Println("Sending client ID:", clientID)
	sendMessage(ws, Message{
		Type: "clientId",
		Data: map[string]string{"clientId": clientID},
	})

	updateUserList()

	// Listen for messages from the client
	for {
		_, msgData, err := ws.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			removeUser(clientID)
			break
		}

		var msg Message
		err = json.Unmarshal(msgData, &msg)
		if err != nil {
			log.Printf("Error parsing message: %v. Data: %s", err, string(msgData))
			continue
		}

		log.Printf("Message received from client %s: %v", clientID, msg)

		// Handle different message types
		switch msg.Type {
		case "mediaOffer":
			log.Printf("Handling mediaOffer from %s", clientID)
			handleMediaOffer(clientID, msg)
		case "mediaAnswer":
			log.Printf("Handling mediaAnswer from %s", clientID)
			handleMediaAnswer(clientID, msg)
		case "iceCandidate":
			log.Printf("Handling iceCandidate from %s", clientID)
			handleICECandidate(clientID, msg)
		case "requestUserList":
			log.Printf("Request for user list from %s", clientID)
			updateUserList()
		case "hangup":
			log.Printf("Handling hangup from %s", clientID)
			handleHangup(clientID, msg)
		default:
			log.Printf("Unknown message type: %v", msg.Type)
		}
	}

	// Remove the client when the connection is closed
	usersMutex.Lock()
	delete(users, clientID)
	usersMutex.Unlock()
	log.Printf("Client %s disconnected", clientID)
}

func handleMediaOffer(fromClientID string, msg Message) {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	toClientID := msg.To
	if user, ok := users[toClientID]; ok {
		log.Printf("Sending mediaOffer from %s to %s", fromClientID, toClientID)
		sendMessage(user.Connection, msg)

		// Log call start
		call := Call{
			From:      fromClientID,
			To:        toClientID,
			StartTime: time.Now(),
			EndTime:   time.Time{}, // Initialize with zero time
			Duration:  0,
		}
		result, err := callsCollection.InsertOne(context.Background(), call)
		if err != nil {
			log.Printf("Error logging call start: %v", err)
		} else {
			log.Printf("Call start logged: %s to %s. MongoDB Document ID: %v", fromClientID, toClientID, result.InsertedID)
			log.Printf("Call details: %+v", call)
		}
	} else {
		log.Printf("Failed to find user with ID: %s to forward mediaOffer", toClientID)
	}
}

func handleMediaAnswer(fromClientID string, msg Message) {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	toClientID := msg.To
	if user, ok := users[toClientID]; ok {
		log.Printf("Sending mediaAnswer from %s to %s", fromClientID, toClientID)
		sendMessage(user.Connection, msg)
	} else {
		log.Printf("Failed to find user with ID: %s to forward mediaAnswer", toClientID)
	}
}

func handleICECandidate(fromClientID string, msg Message) {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	toClientID := msg.To
	if user, ok := users[toClientID]; ok {
		log.Printf("Sending iceCandidate from %s to %s", fromClientID, toClientID)
		sendMessage(user.Connection, msg)
	} else {
		log.Printf("Failed to find user with ID: %s", toClientID)
	}
}

func removeUser(clientID string) {
	usersMutex.Lock()
	defer usersMutex.Unlock()
	delete(users, clientID)
	log.Printf("Client %s removed", clientID)
	updateUserList()
}

func updateUserList() {
	log.Printf("Call updateUserList")
	usersMutex.Lock()
	defer usersMutex.Unlock()

	userIds := make([]string, 0, len(users))
	for id := range users {
		userIds = append(userIds, id)
	}

	msg := Message{
		Type: "requestUserList",
		Data: map[string][]string{"userIds": userIds},
	}

	log.Printf("Sending updated user list: %v", userIds)

	for _, user := range users {
		sendMessage(user.Connection, msg)
	}
}

// Send message to a WebSocket connection
func sendMessage(conn *websocket.Conn, msg Message) {
	msgData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error serializing message: %v", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, msgData); err != nil {
		log.Printf("Error sending message: %v", err)
		// Consider removing the user if the connection is broken
		// This might require passing the clientID to this function
	}
}

func generateClientID() string {
	randomIndex := rand.Intn(len(animalNames))
	animalName := animalNames[randomIndex]
	log.Printf("generateClientID %v", animalName)
	return animalName
}

func handleHangup(fromClientID string, msg Message) {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	log.Printf("Handling hangup from %s. Message: %+v", fromClientID, msg)

	toClientID := msg.To
	if toClientID == "" {
		log.Printf("Received hangup message without 'to' field from %s. Attempting to find active call.", fromClientID)
		// Попытка найти активный звонок для fromClientID
		filter := bson.M{
			"$or": []bson.M{
				{"from": fromClientID},
				{"to": fromClientID},
			},
			"end_time": bson.M{"$eq": time.Time{}}, // Check for zero time
		}
		var call Call
		err := callsCollection.FindOne(context.Background(), filter).Decode(&call)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				log.Printf("No active call found for %s", fromClientID)
			} else {
				log.Printf("Error finding call: %v", err)
			}
			return
		}
		// Определяем toClientID на основе найденного звонка
		if call.From == fromClientID {
			toClientID = call.To
		} else {
			toClientID = call.From
		}
		log.Printf("Found active call between %s and %s", fromClientID, toClientID)
	}

	// Отправка сообщения о завершении звонка, если toClientID определен
	if toClientID != "" && toClientID != fromClientID {
		if user, ok := users[toClientID]; ok {
			log.Printf("Sending hangup from %s to %s", fromClientID, toClientID)
			sendMessage(user.Connection, msg)
		} else {
			log.Printf("Failed to find user with ID: %s to forward hangup", toClientID)
		}
	}

	// Обновление записи о звонке
	endTime := time.Now()
	filter := bson.M{
		"$or": []bson.M{
			{"from": fromClientID, "to": toClientID},
			{"from": toClientID, "to": fromClientID},
		},
		"end_time": bson.M{"$eq": time.Time{}}, // Check for zero time
	}

	update := bson.M{
		"$set": bson.M{
			"end_time": endTime,
			"duration": endTime.Sub(time.Time{}).Seconds(), // We'll calculate the actual duration later
		},
	}

	result, err := callsCollection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		log.Printf("Error updating call: %v", err)
		return
	}

	log.Printf("MongoDB update result: %+v", result)

	if result.MatchedCount == 0 {
		log.Printf("No active call found to update for %s and %s", fromClientID, toClientID)
		return
	}

	// Fetch the updated call document
	var updatedCall Call
	err = callsCollection.FindOne(context.Background(), filter).Decode(&updatedCall)
	if err != nil {
		log.Printf("Error fetching updated call: %v", err)
		// Try to fetch without the end_time filter
		filter = bson.M{
			"$or": []bson.M{
				{"from": fromClientID, "to": toClientID},
				{"from": toClientID, "to": fromClientID},
			},
		}
		err = callsCollection.FindOne(context.Background(), filter).Decode(&updatedCall)
		if err != nil {
			log.Printf("Error fetching updated call (second attempt): %v", err)
			return
		}
	}

	// Calculate and update the actual duration
	duration := updatedCall.EndTime.Sub(updatedCall.StartTime).Seconds()
	_, err = callsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": updatedCall.ID},
		bson.M{"$set": bson.M{"duration": duration}},
	)
	if err != nil {
		log.Printf("Error updating call duration: %v", err)
	}

	log.Printf("Call between %s and %s ended and logged. Duration: %.2f seconds", fromClientID, toClientID, duration)
	log.Printf("Updated call details: %+v", updatedCall)
}

type Call struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	From      string             `bson:"from"`
	To        string             `bson:"to"`
	StartTime time.Time          `bson:"start_time"`
	EndTime   time.Time          `bson:"end_time"`
	Duration  float64            `bson:"duration"`
}

// Add this function to periodically log database statistics
func logDatabaseStats() {
	for {
		time.Sleep(5 * time.Minute) // Log every 5 minutes
		stats, err := callsCollection.EstimatedDocumentCount(context.Background())
		if err != nil {
			log.Printf("Error getting database stats: %v", err)
		} else {
			log.Printf("Current number of documents in calls collection: %d", stats)
		}
	}
}

// Add this function to log all calls (for debugging purposes)
func logAllCalls() {
	cursor, err := callsCollection.Find(context.Background(), bson.M{})
	if err != nil {
		log.Printf("Error fetching all calls: %v", err)
		return
	}
	defer cursor.Close(context.Background())

	var calls []Call
	if err = cursor.All(context.Background(), &calls); err != nil {
		log.Printf("Error decoding calls: %v", err)
		return
	}

	log.Printf("Total calls in database: %d", len(calls))
	for _, call := range calls {
		log.Printf("Call: %+v", call)
	}
}
