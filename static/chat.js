function sendMessage() {
    let text = document.getElementById("inputMessage").value;
    document.getElementById("inputMessage").value = "";
    fetch('/send', {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group: "group1", message: text })
    });
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
    const messageData = JSON.parse(message);

    // message container
    let messageElement = document.createElement("div");
    messageElement.classList.add("message");

    // username container
    let usernameElement = document.createElement("span");
    usernameElement.innerText = messageData.Username;
    usernameElement.style.fontWeight = "bold";
    usernameElement.style.color = "#4CAF50"; // colore verde per il nome utente

    // message content container
    let messageContentElement = document.createElement("span");
    messageContentElement.innerText = " " +messageData.Msg;
    messageContentElement.style.color = "#333"; // colore grigio scuro per il messaggio

    messageElement.appendChild(usernameElement);
    messageElement.appendChild(messageContentElement);

    document.getElementById("messages").appendChild(messageElement);
    scrollToBottom();
}

function scrollToBottom() {
    let chat = document.getElementById("messages");
    chat.scrollTop = chat.scrollHeight;
}
