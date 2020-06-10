const url = "ws://localhost:8000/chat";
const webSocket = new WebSocket(url);
const chatView = document.getElementById("chat-view");
const chatrooms = document.getElementById("chatrooms");
const addNewChatroom = document.getElementById("new-chatroom");
const connectChatroom = document.getElementById("connect-chatroom");

let anonymousName = "";
let chatroomId = "";
let userChatrooms = new Map();
let activeChatroom = "";

// function registerClient() {
//   const formData = new FormData();
//   formData.append("user", anonymousName);

//   fetch("register-client", {
//     method: "POST",
//     mode: "same-origin",
//     body: formData,
//   })
//     .then((res) => {
//       console.log(res);
//     });
// }

function main() {
  const userMessage = document.getElementById("user-message");
  const sendButton = document.getElementById("send");

  addNewChatroom.addEventListener("submit", async (event) => {
    event.preventDefault();

    const formData = new FormData(addNewChatroom);

    let jsonPayload = await fetch("/create-room", {
      method: "POST",
      mode: "same-origin",
      body: formData,
    }).catch((err) => console.log(err));
    let chatroomId = await jsonPayload.json().catch((err) => console.log(err));
    console.log(chatroomId);
  });

  connectChatroom.addEventListener("submit", async (event) => {
    event.preventDefault();

    const formData = new FormData(connectChatroom);
    formData.append("user", anonymousName);

    const jsonPayload = await fetch("/connect-room", {
      method: "POST",
      mode: "same-origin",
      body: formData,
    }).catch((err) => console.log(err));

    const chatroomName = await jsonPayload.json().catch((err) => console.log(err));
    console.log(chatroomName);
    activeChatroom = chatroomName;

    let chatviewNode = document.createElement("div");
    let textNode = document.createTextNode("chatroom 1");
    chatviewNode.appendChild(textNode);
    chatrooms.appendChild(chatviewNode);
  });

  sendButton.addEventListener("click", (event) => {
    event.preventDefault();

    const userMessage = document.getElementById("user-message");
    webSocket.send(JSON.stringify({
      message: userMessage.value,
      messageType: "text",
      chatroomId: activeChatroom,
      id: anonymousName,
    }));
    // console.log(`sent message ${userMessage.value}`);
    userMessage.value = "";
  });

  webSocket.addEventListener("message", (messageJson) => {
    const message = JSON.parse(messageJson.data);

    console.log("message: ", message);

    if (message.MessageType === "message") {
      console.log("message", message.Message);
      const messageNode = document.createElement("p");
      messageNode.appendChild(document.createTextNode(message.Message));
      messageNode.classList.add("chat-bubble");
      chatView.appendChild(messageNode);
    } else if (message.MessageType === "id") {
      anonymousName = message.Id;
    }
  })

}

main();