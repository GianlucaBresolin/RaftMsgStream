function sendMessage() {
    let text = document.getElementById("inputMessage").value;
    document.getElementById("inputMessage").value = "";
    fetch('/send', {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group: "group1", message: text })
    });
}

var username = null;

async function getUsername() {
    const response = await fetch("http://localhost:8080/get-username");
    const data = await response.json();
    console.log(data);
    username = data.value;
}

const socket = new WebSocket("ws://localhost:8080/ws");

socket.onopen = () => {
    console.log("WebSocket connection established");
};

socket.onerror = (error) => {
    console.error("WebSocket error:", error);
};

socket.onmessage = (event) => {
    console.log("Received message:", event.data);
    receiveMessage(event.data);
};

function receiveMessage(message) {
    // retrieve the username if necessary
    if (username === null || username === undefined) {
        getUsername();
    }
    const messageData = JSON.parse(message);

    // message container
    let messageElement = document.createElement("div");
    messageElement.classList.add("message");

    // username container
    let usernameElement = document.createElement("span");
    usernameElement.classList.add("username");
    if (messageData.Username == username) {
        usernameElement.classList.add("our-user");
    } else {
        usernameElement.classList.add("other-user");
    }
    usernameElement.innerText = messageData.Username;

    // message content container
    let messageContentElement = document.createElement("span");
    messageContentElement.classList.add("message-content");
    messageContentElement.innerText = " " +messageData.Msg;

    messageElement.appendChild(usernameElement);
    messageElement.appendChild(messageContentElement);

    document.getElementById("messages").appendChild(messageElement);
    scrollToBottom();
}

function scrollToBottom() {
    let chat = document.getElementById("messages");
    chat.scrollTop = chat.scrollHeight;
}
