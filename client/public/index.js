// Змінні для зберігання важливої інформації
let clientId, selectedUser, remoteOffer, peer;

// Функція для створення RTCPeerConnection
function createPeerConnection() {
  try {
    peer = new RTCPeerConnection({
      iceServers: [{ urls: "stun:stun.stunprotocol.org" }]
    });
    
    // Додаємо обробники подій безпосередньо при створенні
    peer.addEventListener("track", (event) => {
      console.log("Отримано новий трек:", event.streams);
      const [stream] = event.streams;
      const remoteVideoElement = document.querySelector("#remoteVideo");
      if (remoteVideoElement) {
        remoteVideoElement.srcObject = stream;
        console.log("Додано медіапотік до елементу відеоплеєра");
      } else {
        console.error("Не знайдено елемент #remoteVideo");
      }
    });

    peer.onicecandidate = (event) => {
      if (event.candidate) {
        console.log("Відправка ICE кандидата:", event.candidate);
        sendIceCandidate(event.candidate);
      } else {
        console.log("Усі ICE кандидати були відправлені");
      }
    };

    peer.oniceconnectionstatechange = () => {
      console.log("ICE connection state:", peer.iceConnectionState);
      if (peer.iceConnectionState === "connected") {
        console.log("З'єднання встановлено!");
      } else if (peer.iceConnectionState === "failed") {
        console.error("Не вдалося встановити з'єднання!");
      }
    };

    peer.onicegatheringstatechange = () => {
      console.log("ICE Gathering State Change:", peer.iceGatheringState);
    };
    
    console.log("RTCPeerConnection створено");
  } catch (error) {
    console.error("Помилка створення RTCPeerConnection:", error);
  }
}

// Створюємо з'єднання при ініціалізації
createPeerConnection();

// Підключення до WebSocket серверу
const socket = new WebSocket(`wss://${window.location.hostname}:8080/ws`);

// Обробник відкриття WebSocket з'єднання
socket.onopen = async () => {
  console.log("WebSocket з'єднання встановлено");
  await onSocketConnected();
  console.log("Запитую список користувачів");
  socket.send(JSON.stringify({ type: "requestUserList" }));
};

// Обробка повідомлень через WebSocket
socket.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log("Отримано повідомлення:", data.type);

  const handlers = {
    "update-user-list": () => console.log("Оновлено список користувачів:", data.data.userIds),
    clientId: () => {
      clientId = data.data.clientId;
      document.querySelector("#userId").innerHTML = `Мій ID: ${clientId}`;
    },
    requestUserList: () => onUpdateUserList(data.data),
    mediaOffer: () => {
      selectedUser = data.from;
      handleMediaOffer(data);
    },
    mediaAnswer: handleMediaAnswer,
    iceCandidate: handleIceCandidate,
    hangup: endCall
  };

  if (handlers[data.type]) {
    handlers[data.type](data);
  } else {
    console.log("Невідомий тип повідомлення:", data.type);
  }
};

// Обробка медіапропозиції (offer)
async function handleMediaOffer(data) {
  try {
    if (!data.offer || !data.offer.type || !data.offer.sdp) {
      throw new Error("Некоректна медіапропозиція (offer)");
    }

    await peer.setRemoteDescription(new RTCSessionDescription(data.offer));
    console.log("Встановлено remote offer");

    const peerAnswer = await peer.createAnswer();
    await peer.setLocalDescription(peerAnswer);

    console.log("Відправляю answer");
    sendMediaAnswer(peerAnswer);
  } catch (error) {
    console.error("Помилка при встановленні remote offer або створенні answer:", error);
  }
}

// Відправка медіавідповіді (answer)
function sendMediaAnswer(answer) {
  if (selectedUser) {
    socket.send(JSON.stringify({
      type: "mediaAnswer",
      answer: { type: answer.type, sdp: answer.sdp },
      from: clientId,
      to: selectedUser
    }));
    console.log(`Відправляю mediaAnswer від ${clientId} до ${selectedUser}`);
  } else {
    console.error("Не встановлено отримувача для mediaAnswer");
  }
}

// Обробка медіапідтвердження (answer)
async function handleMediaAnswer(data) {
  try {
    if (!data.answer || !data.answer.type || !data.answer.sdp) {
      throw new Error("Некоректне медіапідтвердження (answer)");
    }

    await peer.setRemoteDescription(new RTCSessionDescription(data.answer));
    console.log("Встановлено remote answer");
  } catch (error) {
    console.error("Помилка при встановленні remote answer:", error);
  }
}

// Обробка ICE кандидатів
function handleIceCandidate(data) {
  try {
    if (!data.candidate || !data.candidate.candidate) {
      throw new Error("Некоректний ICE кандидат");
    }

    const candidate = new RTCIceCandidate(data.candidate);
    peer.addIceCandidate(candidate)
      .then(() => console.log("ICE кандидат додано:", candidate))
      .catch(error => console.error("Помилка при додаванні ICE кандидата:", error));
  } catch (error) {
    console.error("Помилка при обробці ICE кандидата:", error);
  }
}

// Відправка ICE кандидата
function sendIceCandidate(candidate) {
  socket.send(JSON.stringify({
    type: "iceCandidate",
    candidate: {
      candidate: candidate.candidate,
      sdpMid: candidate.sdpMid,
      sdpMLineIndex: candidate.sdpMLineIndex
    },
    from: clientId,
    to: selectedUser
  }));
  console.log(`Відправляю ICE-кандидата від ${clientId} до ${selectedUser}`);
}

// Оновлення списку користувачів
function onUpdateUserList({ userIds }) {
  const usersList = document.querySelector("#usersList");

  if (!Array.isArray(userIds)) {
    console.error("userIds не визначено або не є масивом", userIds);
    return;
  }

  const usersToDisplay = userIds.filter(id => id !== clientId);

  usersList.innerHTML = usersToDisplay.length ? "" : "<div>Немає інших підключених користувачів</div>";

  usersToDisplay.forEach(user => {
    const userItem = document.createElement("div");
    userItem.textContent = user;
    userItem.className = "user-item";
    userItem.addEventListener("click", () => selectUser(user, userItem));
    usersList.appendChild(userItem);
  });
}

// Вибір користувача для дзвінка
function selectUser(user, userItem) {
  document.querySelectorAll(".user-item").forEach(element => {
    element.classList.remove("user-item--touched");
  });
  userItem.classList.add("user-item--touched");
  selectedUser = user;
  console.log("Вибрано користувача для дзвінка:", selectedUser);
}

// Обробка підключення до WebSocket
async function onSocketConnected() {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: true });
    document.querySelector("#localVideo").srcObject = stream;
    stream.getTracks().forEach(track => peer.addTrack(track, stream));
    console.log("Медіа потік отримано та додано до peer connection");
  } catch (error) {
    console.error("Помилка при отриманні медіапотоку:", error);
    // Тут можна додати обробку помилки, наприклад, показати повідомлення користувачу
  }
}

// Обробка натискання кнопки виклику
document.querySelector("#call").addEventListener("click", async () => {
  if (selectedUser) {
    console.log("Виклик користувача:", selectedUser);
    const localPeerOffer = await peer.createOffer();
    await peer.setLocalDescription(localPeerOffer);

    console.log("Відправка медіапропозиції (offer)");
    sendMediaOffer(localPeerOffer);
  } else {
    console.log("Користувач не вибраний для виклику");
  }
});

// Відправка медіапропозиції (offer)
function sendMediaOffer(offer) {
  socket.send(JSON.stringify({
    type: "mediaOffer",
    offer: { type: offer.type, sdp: offer.sdp },
    from: clientId,
    to: selectedUser
  }));
}

// Обробка натискання на кнопку завершення виклику
document.querySelector("#hangup").addEventListener("click", () => {
  console.log("Завершення виклику");
  endCall();
  socket.send(JSON.stringify({
    type: "hangup",
    from: clientId,
    to: selectedUser
  }));
});

// Функція завершення виклику
function endCall() {
  if (peer) {
    peer.getSenders().forEach(sender => {
      sender.track.stop();
    });
    peer.close();
    peer = null;
  }
  ["#localVideo", "#remoteVideo"].forEach(selector => {
    const videoElement = document.querySelector(selector);
    if (videoElement.srcObject) {
      videoElement.srcObject.getTracks().forEach(track => track.stop());
      videoElement.srcObject = null;
    }
  });
  selectedUser = null;
  console.log("Виклик завершено");
}

// Добавьте эту функцию в конец файла
function fetchCallLogs() {
    fetch('/calls')
        .then(response => response.json())
        .then(calls => {
            const callLogsElement = document.querySelector("#callLogs");
            callLogsElement.innerHTML = '';
            calls.forEach(call => {
                const callElement = document.createElement('div');
                callElement.className = 'list-group-item';
                const duration = call.duration ? `${Math.round(call.duration)} seconds` : 'N/A';
                callElement.innerHTML = `
                    <strong>From:</strong> ${call.from}<br>
                    <strong>To:</strong> ${call.to}<br>
                    <strong>Start:</strong> ${new Date(call.start_time).toLocaleString()}<br>
                    <strong>End:</strong> ${call.end_time ? new Date(call.end_time).toLocaleString() : 'N/A'}<br>
                    <strong>Duration:</strong> ${duration}
                `;
                callLogsElement.appendChild(callElement);
            });
        })
        .catch(error => console.error('Error fetching call logs:', error));
}

// Добавьте этот обработчик событий
document.querySelector("#showCallLogs").addEventListener("click", fetchCallLogs);
