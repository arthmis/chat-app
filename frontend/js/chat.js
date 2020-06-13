const url = "ws://localhost:8000/ws";
const webSocket = new WebSocket(url);
// const chatView = document.getElementById("chat-view");
// const chatrooms = document.getElementById("chatrooms");
// const connectChatroom = document.getElementById("connect-chatroom");

let username = "art";
let activeChatroom = "";
// let userChatrooms = new Map();

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
  const addNewChatroom = document.getElementById("new-chatroom");
  const newChatroomFormWrapper = document.getElementById("new-chatroom-form-wrapper");
  const newChatroomForm = document.getElementById("new-chatroom-form");

  newChatroomFormWrapper.addEventListener("click", (event) => {
    if (event.target === newChatroomFormWrapper) {
      newChatroomFormWrapper.classList.toggle("visibility");
    }
  });

  addNewChatroom.addEventListener("click", (event) => {
    event.preventDefault();
    newChatroomFormWrapper.classList.toggle("visibility");
  })

  newChatroomForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const formData = new FormData(newChatroomForm);

    if (!newChatroomForm.checkValidity()) {
      return;
    }

    let res = await fetch("/create-room", {
      method: "POST",
      body: formData,
      mode: "same-origin",
    });

    let data = await res.json();
    activeChatroom = data;


  });

  // connectChatroom.addEventListener("submit", async (event) => {
  //   event.preventDefault();

  //   const formData = new FormData(connectChatroom);
  //   formData.append("user", anonymousName);

  //   const jsonPayload = await fetch("/connect-room", {
  //     method: "POST",
  //     mode: "same-origin",
  //     body: formData,
  //   }).catch((err) => console.log(err));

  //   const chatroomName = await jsonPayload.json().catch((err) => console.log(err));
  //   console.log(chatroomName);
  //   activeChatroom = chatroomName;

  //   let chatviewNode = document.createElement("div");
  //   let textNode = document.createTextNode("chatroom 1");
  //   chatviewNode.appendChild(textNode);
  //   chatrooms.appendChild(chatviewNode);
  // });

  sendButton.addEventListener("click", (event) => {
    event.preventDefault();

    const userMessage = document.getElementById("user-message");
    webSocket.send(JSON.stringify({
      message: userMessage.value,
      messageType: "text",
      chatroomName: activeChatroom,
      user: username,
    }));
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
      username = message.Id;
    }
  })

}

main();