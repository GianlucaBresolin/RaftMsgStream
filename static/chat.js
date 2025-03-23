function handleMembership() {
    if (sessionStorage.getItem("membership") === "true") {
        document.getElementById("chatContainer").style.display = "flex";
        document.getElementById("leaveGroup").style.display = "block";
        document.getElementById("joinGroup").style.display = "none";
    } else {
        document.getElementById("chatContainer").style.display = "none";
        document.getElementById("leaveGroup").style.display = "none";
        document.getElementById("joinGroup").style.display = "block";
    }
}

var eventSource = null;

window.onload = async () => {
    if (sessionStorage.getItem("username") != null) {
        document.getElementById("loginContainer").style.display = "none";
        handleMembership();
    } else {
        document.getElementById("loginContainer").style.display = "flex";
        document.getElementById("chatContainer").style.display = "none";
        document.getElementById("leaveGroup").style.display = "none";
        document.getElementById("joinGroup").style.display = "none";
    }
}

window.onclose = async () => { 
    fetch('/unsubscribe', {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ 
            user: sessionStorage.getItem("username"),
        })
    }).then(() => {
        eventSource.close();
    })
}

function loginUser() {
    sessionStorage.setItem("username", document.getElementById("username").value);
    sessionStorage.setItem("USN", 0);

    document.getElementById("loginContainer").style.display = "none";
    handleMembership();
}

async function joinGroup(url = '/send') {
    let response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ 
            user: sessionStorage.getItem("username"),
            USN: parseInt(sessionStorage.getItem("USN")),
            group: "RaftMsgStream", 
            message: null })
    });

    let data = response.json();

    if (response.status == 200) {
        sessionStorage.setItem("membership", "true");
        sessionStorage.setItem("lastMessageIndex", -1);
        sessionStorage.setItem("USN", parseInt(sessionStorage.getItem("USN")) + 1);
        handleMembership();
    } else {
        if (data.leader != null) {
            await joinGroup(`http://${data.leader}/send`);
        }
    }

    fetch('/subscribe', {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ 
            user: sessionStorage.getItem("username"),
        })
    }).then(() => {
        eventSource = new EventSource('/publish?user=' + sessionStorage.getItem("username"));

        eventSource.onmessage =  function(event) {
            updateMessages();
        }
    });
}

async function sendMessage(url = '/send') {
    let message = document.getElementById("inputMessage").value;
    document.getElementById("inputMessage").value = "";
    sessionStorage.setItem("USN", parseInt(sessionStorage.getItem("USN")) + 1);
    previousUSN = parseInt(sessionStorage.getItem("USN"))-1;
    let response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ 
            user: sessionStorage.getItem("username"),
            USN: previousUSN,
            group: "RaftMsgStream", 
            message: message })
    });

    let data = response.json();
    
    if (data.status == 'redirecting to leader') {
        await sendMessage(`http://${data.leader}/send`);
    }
}

async function leaveGroup(url = '/leave') {
    let response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ 
            user: sessionStorage.getItem("username"),
            USN: parseInt(sessionStorage.getItem("USN")),
            group: "RaftMsgStream" })
    });
    
    let data = response.json();

    if (response.status == 200) {
        sessionStorage.setItem("membership", "false");
        sessionStorage.setitem("lastMessageIndex", -1);
        sessionStorage.setItem("USN", partseInt(sessionStorage.getItem("USN")) + 1);
        handleMembership();
    } else {
        if (data.leader != null) {
            await leaveGroup(`http://${data.leader}/leave`);
        }
    }
}

async function updateMessages(url = '/update') {
    // we have been notified by the server, we need to update our messages
    let response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ 
            user: sessionStorage.getItem("username"),
            USN: parseInt(sessionStorage.getItem("USN")),
            lastMessageIndex: parseInt(sessionStorage.getItem("lastMessageIndex")),
            group: "RaftMsgStream" })
    });

    let result = await response.json();

    if (result.status == 'state updated') {
        let messages = result.state.Messages;
        messages.forEach((message) => {
            receiveMessage(message);
            sessionStorage.setItem("lastMessageIndex", parseInt(sessionStorage.getItem("lastMessageIndex")) + 1);
        });
    } else if (result.status == 'redirecting to leader') {
        await updateMessages(`http://${result.leader}/update`);
    }
}

function receiveMessage(message) {
    // message container
    let messageElement = document.createElement("div");
    messageElement.classList.add("message");

    // username container
    let usernameElement = document.createElement("span");
    usernameElement.classList.add("username");
    if (message.Username == sessionStorage.getItem("username")) {
        usernameElement.classList.add("our-user");
    } else {
        usernameElement.classList.add("other-user");
    }
    usernameElement.innerText = message.Username;

    // message content container
    let messageContentElement = document.createElement("span");
    messageContentElement.classList.add("message-content");
    messageContentElement.innerText = " " +message.Msg;

    messageElement.appendChild(usernameElement);
    messageElement.appendChild(messageContentElement);

    document.getElementById("messages").appendChild(messageElement);
    scrollToBottom();
}

function scrollToBottom() {
    let chat = document.getElementById("messages");
    chat.scrollTop = chat.scrollHeight;
}