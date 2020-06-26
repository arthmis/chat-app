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



  const joinChatroom = document.getElementById("join-chatroom");
  const joinChatroomFormWrapper = document.getElementById("join-chatroom-form-wrapper");
  const joinChatroomForm = document.getElementById("join-chatroom-form");

  joinChatroomFormWrapper.addEventListener("click", (event) => {
    if (event.target === joinChatroomFormWrapper) {
      joinChatroomFormWrapper.classList.toggle("visibility");
    }
  });

  joinChatroom.addEventListener("click", (event) => {
    event.preventDefault();
    joinChatroomFormWrapper.classList.toggle("visibility");
  });

  joinChatroomForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const formData = new FormData(joinChatroomForm);

    if (!joinChatroomForm.checkValidity()) {
      return;
    }

    let res = await fetch("/join-room", {
      method: "POST",
      body: formData,
      mode: "same-origin",
    });

    if (res.status !== 202) {
      console.error("Problem joining chatrooms");
      return
    }

    let data = await res.json();
    activeChatroomName = data;
    userChatrooms[activeChatroomName] = activeChatroomName;
    joinChatroomFormWrapper.classList.toggle("visibility");

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

    // <div id="chat-menu">
    //   <button id="create-invite">Create Invite</button>

    // </div>

    const chatMenu = document.createElement("div");
    const createInviteButton = document.createElement("button");
    createInviteButton.classList.add("create-invite");
    createInviteButton.appendChild(document.createTextNode("Create Invite"));
    createInviteButton.addEventListener("click", (event) => {
      event.preventDefault();
      if (activeChatroom === null) {
        createInviteFormWrapper.classList.toggle("visibility");
        return;
      }
      createInviteFormWrapper.classList.toggle("visibility");
    });

    chatMenu.appendChild(createInviteButton);

    chatMenu.classList.add("chat-menu");
    const newView = document.createElement("div");
    newView.classList.add("chat-view");
    newView.insertBefore(chatMenu, newView.firstChild);
    chatrooms[data] = new Chatroom(data, newView);
    // chatView.prepend(chatMenu);
    chatView.appendChild(newView);

    // addInviteListener(createInviteButton);

  });

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
  });

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

    const createInviteButton = document.createElement("button");
    createInviteButton.classList.add("create-invite");
    createInviteButton.appendChild(document.createTextNode("Create Invite"));
    createInviteButton.addEventListener("click", (event) => {
      event.preventDefault();
      if (activeChatroom === null) {
        createInviteFormWrapper.classList.toggle("visibility");
        return;
      }
      createInviteFormWrapper.classList.toggle("visibility");
    });

    const chatMenu = document.createElement("div");
    chatMenu.classList.add("chat-menu");
    chatMenu.appendChild(createInviteButton);

    const newView = document.createElement("div");
    newView.classList.add("chat-view");
    newView.insertBefore(chatMenu, newView.firstChild)
    chatrooms[data] = new Chatroom(data, newView);
    // chatView.prepend(chatMenu);
    chatView.appendChild(newView);


    // addInviteListener(createInviteButton);

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

  const sendButton = document.getElementById("send");

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

const createInviteFormWrapper = document.getElementById("create-invite-form-wrapper");
const createInviteForm = document.getElementById("create-invite-form");

createInviteFormWrapper.addEventListener("click", (event) => {
  if (event.target === createInviteFormWrapper) {
    createInviteFormWrapper.classList.toggle("visibility");
  }
});
createInviteForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const formData = new FormData(createInviteForm);
  formData.append("chatroom_name", activeChatroom.textContent);

  if (!createInviteForm.checkValidity()) {
    return;
  }

  let res = await fetch("/create-invite", {
    method: "POST",
    body: formData,
    mode: "same-origin",
  });

  if (res.status !== 202) {
    console.log("Error creating invite");
  }
  const inviteCode = await res.json();
  console.log(inviteCode);
  createInviteFormWrapper.classList.toggle("visibility");

  // const inviteCodeElement = document.createElement("div");
  // inviteCodeElement.appendChild(document.createTextNode(inviteCode));
  const inviteCodeWrapper = document.getElementById("invite-code-wrapper");
  inviteCodeWrapper.classList.toggle("visibility");
  document.getElementById("invite-code").value = inviteCode;
});

const inviteCodeWrapper = document.getElementById("invite-code-wrapper");

inviteCodeWrapper.addEventListener("click", (event) => {
  if (event.target === inviteCodeWrapper) {
    inviteCodeWrapper.classList.toggle("visibility");
  }
});

document.getElementById("back-create-invite").addEventListener("click", (event) => {
  event.preventDefault();
  inviteCodeWrapper.classList.toggle("visibility");
  createInviteFormWrapper.classList.toggle("visibility");
});

function addInviteListener(createInvite) {



}

main();