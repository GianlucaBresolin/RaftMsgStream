function sendMessage() {
    let text = document.getElementById("inputMessage").value;
    document.getElementById("inputMessage").value = "";
    fetch('/send', {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group: "RaftMsgStream", message: text })
    });
}

var username; 

async function getUsername() {
    while (username == null || username == undefined) {
        const response = await fetch("http://localhost:8080/get-username");
        const data = await response.json();
        username = data.value;
    }
}

var membership = false;
async function getMembership() {
    const response = await fetch('/get-membership', {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group: "RaftMsgStream" })
    });
    const data = await response.json();
    membership = data.membership;
}

function joinGroup() {
    fetch('/send', {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group: "RaftMsgStream", message: null })
    });

    setTimeout(async () => {
        if (membership == false || membership == undefined) {
            await getMembership();
            if (membership == true) {
                document.getElementById("chatContainer").style.display = "flex";
                document.getElementById("leaveGroup").style.display = "block";
                document.getElementById("joinGroup").style.display = "none";
            }
        }
    }, 300);
}

function leaveGroup() {
    fetch('/leave', {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group: "RaftMsgStream" })
    });
    
    setTimeout(async () => {
        if (membership == true) {
            await getMembership();
            if (membership == false) {
                document.getElementById("chatContainer").style.display = "none";
                document.getElementById("leaveGroup").style.display = "none";
                document.getElementById("joinGroup").style.display = "block";
                document.getElementById("messages").innerHTML = "";
            }
        }
    }, 300);
}


const socket = new WebSocket("ws://localhost:8080/ws");

socket.onopen = async () => {
    if (username == null || username == undefined) {
        await getUsername();
    }
    if (membership == false) {
        document.getElementById("chatContainer").style.display = "none";
        document.getElementById("leaveGroup").style.display = "none";   
        document.getElementById("joinGroup").style.display = "block";
    }
};

socket.onerror = (error) => {
    console.error("WebSocket error:", error);
};

socket.onmessage = async (event) => {
    receiveMessage(event.data);
};

async function receiveMessage(message) {
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