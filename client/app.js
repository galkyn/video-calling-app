const express = require('express');
const https = require('https');
const fs = require('fs');
const path = require('path');
const { MongoClient } = require('mongodb');

const app = express();
const port = process.env.PORT || 3000;

console.log('Starting the application...');

// Serve static files from the 'public' directory
app.use(express.static(path.join(__dirname, 'public')));
console.log(`Serving static files from: ${path.join(__dirname, 'public')}`);

// Function to connect to MongoDB
async function connectToMongo() {
    const uri = process.env.MONGO_URI || "mongodb://videoCallUser:password@mongodb:27017/videoCallDB?authSource=videoCallDB";
    console.log('Attempting to connect to MongoDB with URI:', uri);
    const client = new MongoClient(uri);
    try {
        await client.connect();
        console.log('Successfully connected to MongoDB');
        return { client, collection: client.db("videoCallDB").collection("calls") };
    } catch (error) {
        console.error('Failed to connect to MongoDB:', error);
        throw error;
    }
}

// Route to fetch call logs
app.get('/calls', async (req, res) => {
    console.log('Received request for call logs');
    let client;
    try {
        const { client: mongoClient, collection } = await connectToMongo();
        client = mongoClient;
        console.log('Connected to MongoDB, fetching call logs...');
        const calls = await collection.find({}).sort({ start_time: -1 }).limit(5).toArray();
        console.log(`Retrieved ${calls.length} call logs`);
        res.json(calls);
    } catch (error) {
        console.error('Error fetching calls:', error);
        res.status(500).json({ error: 'Internal server error' });
    } finally {
        if (client) {
            await client.close();
            console.log('MongoDB connection closed');
        }
    }
});

// Serve index.html for all other routes
app.get('*', (req, res) => {
    console.log(`Received request for: ${req.url}`);
    res.sendFile(path.join(__dirname, 'public', 'index.html'));
});

const options = {
    key: fs.readFileSync('/app/certs/private.key'),
    cert: fs.readFileSync('/app/certs/certificate.crt')
};

console.log('SSL certificates loaded');

https.createServer(options, app).listen(port, () => {
    console.log(`Node.js HTTPS server is running on https://localhost:${port}`);
});

process.on('unhandledRejection', (reason, promise) => {
    console.error('Unhandled Rejection at:', promise, 'reason:', reason);
});

process.on('uncaughtException', (error) => {
    console.error('Uncaught Exception:', error);
    process.exit(1);
});

console.log('Application setup complete');
