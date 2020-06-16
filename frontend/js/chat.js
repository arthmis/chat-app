const url = "ws://localhost:8000/ws";
const webSocket = new WebSocket(url);
const chatView = document.getElementById("chat-view");
const chatrooms = document.getElementById("chatrooms");
// const connectChatroom = document.getElementById("connect-chatroom");

let username = "art";
let activeChatroomName = "";
let activeChatroom = null;
let userChatrooms = new Map();
const chatviewContent = null;

class Chatroom {
  constructor(name, view) {
    this.name = name;
    this.view = view;
  }
}
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
    activeChatroomName = data;
    userChatrooms[activeChatroomName] = activeChatroomName;
    newChatroomFormWrapper.classList.toggle("visibility");

    if (activeChatroom !== null) {
      activeChatroom.classList.toggle("active-chatroom");
      chatrooms[activeChatroom.textContent].view.hidden = true;
    }

    activeChatroom = document.createElement("div");
    activeChatroom.appendChild(document.createTextNode(activeChatroomName));
    activeChatroom.classList.add("chatroom-name", "active-chatroom");

    activeChatroom.addEventListener("click", (event) => {
      event.preventDefault();
      if (activeChatroom !== null) {
        activeChatroom.classList.toggle("active-chatroom");
        chatrooms[activeChatroom.textContent].view.hidden = true;
      }
      activeChatroom = event.target;
      activeChatroom.classList.toggle("active-chatroom");
      chatViewContent = chatrooms[event.target.textContent];
      chatViewContent.view.hidden = false;
    });
    chatrooms.appendChild(activeChatroom);

    const newView = document.createElement("div");
    newView.classList.add("chat-view");
    chatrooms[data] = new Chatroom(data, newView);
    chatView.appendChild(newView);

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
      chatroomName: activeChatroom.textContent,
      user: username,
    }));
    userMessage.value = "";
  });

  webSocket.addEventListener("message", (messageJson) => {
    const message = JSON.parse(messageJson.data);

    if (messageJson.type === "message") {
      const messageNode = document.createElement("div");
      messageNode.appendChild(document.createTextNode(message.Message));
      messageNode.classList.add("chat-bubble");
      chatrooms[message.ChatroomName].view.appendChild(messageNode);
    }
  })

}

main();