
/* eslint-disable func-names */
/* eslint-disable no-await-in-loop */
/* eslint-disable no-console */
/* eslint-disable prefer-arrow-callback */
import React from 'react';
import ReactDOM from 'react-dom';

class JoinRoomForm extends React.Component {
    render() {
        return (
            <div id="join-chatroom-form-wrapper" onClick={this.props.closeModal}>
                <form id="join-chatroom-form" action="/join-room" method="POST">
                    <p>Join a new community</p>
                    <div className="input-group">
                        <label for="join-chatroom-name">Invite Code</label>
                        <input className="user-input" type="text" id="join-invite-code" name="invite_code"
                            form="join-chatroom-form" placeholder="Unique Name" required />
                    </div>
                    <input type="submit" form="join-chatroom-form" id="join-chatroom-submit" value="Join" />
                </form>
            </div>
        )
    }
}
class NewRoomForm extends React.Component {
    render() {
        return (
            // <div id="new-chatroom-form-wrapper" className="visibility" >
            <div id="new-chatroom-form-wrapper" onClick={this.props.closeModal}>
                <form id="new-chatroom-form" onSubmit={this.props.submit} action="/create-room" method="POST">
                    <p>Create your new chatroom</p>
                    <div className="input-group">
                        <label for="new-chatroom-name">Chatroom Name</label>
                        <input className="user-input" type="text" id="new-chatroom-name" name="chatroom_name"
                            form="new-chatroom-form" placeholder="Unique Name" required />
                    </div>
                    <input type="submit" form="new-chatroom-form" id="new-chatroom-submit" value="Create" />
                </form>
            </div>
        );
    }
}
class Rooms extends React.Component {
    state = {
        activeModal: "",
    }
    closeJoinChatroom = (event) => {
        event.preventDefault();

        const joinRoomFormWrapper = document.getElementById("join-chatroom-form-wrapper");
        if (event.target === joinRoomFormWrapper) {
            this.setState({ activeModal: "" });
        }

    }
    clickJoinChatroom = (event) => {
        event.preventDefault();

        this.setState((prevState, prevProps) => {
            prevState.activeModal = <JoinRoomForm closeModal={this.closeJoinChatroom} />;
            return prevState;
        });

    }
    joinChatroom = (event) => {
        event.preventDefault();
    }
    closeCreateNewChatroom = (event) => {
        const createInviteFormWrapper = document.getElementById("new-chatroom-form-wrapper");
        if (event.target === createInviteFormWrapper) {
            this.setState({ activeModal: "" });
        }
    }
    clickCreateNewChatroom = (event) => {
        event.preventDefault();

        this.setState((prevState, prevProps) => {
            prevState.activeModal = <NewRoomForm submit={this.props.createNewChatroom} closeModal={this.closeCreateNewChatroom} />;
            return prevState;
        });
    }
    render() {
        return (
            <div id="chatrooms-wrapper">
                {/* <form id="connect-chatroom" method="POST">
                    <input id="chatroom-name" type="text" name="room-id" form="connect-chatroom" />
                    <input id="add-chatroom" type="submit" value="Add Chatroom" form="connect-chatroom" />
                </form>  */}
                <div id="chatrooms">
                    {this.props.rooms.map((name) => {
                        if (name === this.props.currentRoom) {
                            return (
                                <div className="chatroom-name active-chatroom">{name}</div>
                            );
                        }
                        return (
                            <div className="chatroom-name">{name}</div>
                        );
                    })}
                </div>
                <button id="join-chatroom" onClick={this.clickJoinChatroom}>Join Chatroom</button>
                <button id="new-chatroom" onClick={this.clickCreateNewChatroom}>New Chatroom</button>
                {this.state.activeModal}
            </div>
        );
    }
}

class Message extends React.Component {
    constructor(props) {
        super(props);
    }
    render() {
        return (
            <div></div>
        );
    }
}

class Messages extends React.Component {
    constructor(props) {
        super(props);
    }


    render() {
        return (
            <div id="chat-view-wrapper">
                <div id="chat-view">

                </div>
                <form id="user-message-wrapper" onSubmit={this.props.sendMessage} method="POST" encType="multipart/form-data">
                    <input id="user-message" type="text" name="user-message" placeholder="type your message" />
                    <input type="submit" id="send" name="send" value="Send" />
                </form>
            </div>
        )
    }
}
class Main extends React.Component {
    constructor(props) {
        super(props);

        this.state = {
            webSocket: new WebSocket("ws://localhost:8000/ws"),
            rooms: [],
            current_room: "",
            current_room_message: [],
        }
        this.sendMessage = this.sendMessage.bind(this);
        this.createNewChatroom = this.createNewChatroom.bind(this);

    }

    async createNewChatroom(event) {
        event.preventDefault();

        let form = document.getElementById("new-chatroom-form");
        const formData = new FormData(form);

        let jsonPayload = await fetch("/create-room", {
            method: "POST",
            mode: "same-origin",
            body: formData,
        }).catch((err) => console.log(err));
        let chatroomId = await jsonPayload.json().catch((err) => console.log(err));
        console.log(chatroomId);
        this.setState((prevState, prevProps) => {
            prevState.current_room = chatroomId;
            // TODO: I should probably have a map to easily look up the room
            // and also check for uniqueness
            // That way if someone leaves a room, it would be easy to delete
            // all messages pertaining to that room using the chatroomId
            prevState.rooms.push(chatroomId);
            return prevState;
        });
    }
    sendMessage(event) {
        event.preventDefault();
        // let form =
        console.log("sending message");

        const userMessage = document.getElementById("user-message");
        this.state.webSocket.send(JSON.stringify({
            message: userMessage.value,
            messageType: "text",
            chatroomName: this.props.currentRoom,
            // user: username,
            user: "art",
        }));
        userMessage.value = "";
    }
    async createChatroom(event) {
    }

    async componentDidMount() {
        // retrieves users chatrooms and their current chatroom
        const response = await fetch("/user/chatrooms", {
            method: 'POST',
            mode: 'same-origin',
        })
        const jsonData = await response.json();
        let chatrooms = jsonData["chatrooms"]
        let currentRoom = jsonData["current_room"]
        // chatrooms will be null if the user is not part of any chatrooms
        if (chatrooms === null) {
            chatrooms = []
        }
        // TODO
        // retrieves messages for current chatroom
        // populates messages component with the most recent messages
        // this will eventually keep the last seen message for the user
        // so they will be in the place they left off
        this.setState({ rooms: chatrooms, current_room: currentRoom })
    }

    render() {
        return (
            <main>
                <Rooms
                    currentRoom={this.state.current_room}
                    rooms={this.state.rooms}
                    createNewChatroom={this.createNewChatroom}
                />
                <Messages
                    messages={this.state.current_room_messages}
                    sendMessage={this.sendMessage}
                    currentRoom={this.state.current_room}
                />
            </main>
        )
    }
}
class App extends React.Component {
    render() {
        return (
            <div>
                <header>
                    <nav>
                        <a id="landing-page" href="/">Chat App</a>
                        <form id="logout" action="/logout" method="POST">
                            <input type="submit" value="Logout" form="logout" />
                        </form>
                    </nav>
                </header>
                <Main />
                {/* <div id="new-chatroom-form-wrapper" class="visibility">
                    <form id="new-chatroom-form" action="/create-room" method="POST">
                        <p>Create your new chatroom</p>
                        <div class="input-group">
                            <label for="new-chatroom-name">Chatroom Name</label>
                            <input class="user-input" type="text" id="new-chatroom-name" name="chatroom_name"
                                form="new-chatroom-form" placeholder="Unique Name" required />
                        </div>
                        <input type="submit" form="new-chatroom-form" id="new-chatroom-submit" value="Create"/>
                    </form>
                </div>
                <div id="join-chatroom-form-wrapper" class="visibility">
                    <form id="join-chatroom-form" action="/join-room" method="POST">
                        <p>Join a new community</p>
                        <div class="input-group">
                            <label for="join-chatroom-name">Invite Code</label>
                            <input class="user-input" type="text" id="join-invite-code" name="invite_code"
                                form="join-chatroom-form" placeholder="Unique Name" required />
                        </div>
                        <input type="submit" form="join-chatroom-form" id="join-chatroom-submit" value="Join" />
                    </form>
                </div>
                <div id="create-invite-form-wrapper" class="visibility">
                    <form id="create-invite-form" action="/create-invite" method="POST">
                        <p>Create Invitation for your community</p>
                        <div class="invitation-group">
                            <input class="invitation-choices" type="radio" id="twenty-four-hours" name="invite_timelimit"
                                form="create-invite-form" value="1 day" required />
                            <label for="twenty-four-hours">1 day</label>
                        </div>
                        <div class="invitation-group">
                            <input class="invitation-choices" type="radio" id="one-week" name="invite_timelimit"
                                form="create-invite-form" value="1 week" required />
                            <label for="one-week">1 week</label>
                        </div>
                        <div class="invitation-group">
                            <input class="invitation-choices" type="radio" id="forever" name="invite_timelimit"
                                form="create-invite-form" value="Forever" required />
                            <label for="forever">Forever</label>
                        </div>
                        <input type="submit" form="create-invite-form" id="create-invite-submit" value="Create Invitation" />
                    </form>
                </div>
                <div id="invite-code-wrapper" class="visibility">
                    <div id="invite-code-view">
                        <div>
                            <button id="back-create-invite">{"<-Back"}</button>
                        </div>
                        <label for="invite-code">Here's your invite code</label>
                        <input id="invite-code" type="text" readonly />
                        <span><button>Copy</button></span>
                    </div>
                </div>*/}
            </div>
        )
    }
}

const root = document.getElementById("root");
ReactDOM.render(<App />, root);
//  const sendButton = document.getElementById("send");

//   sendButton.addEventListener("click", (event) => {
//     event.preventDefault();

//     const userMessage = document.getElementById("user-message");
//     webSocket.send(JSON.stringify({
//       message: userMessage.value,
//       messageType: "text",
//       chatroomName: activeChatroom.textContent,
//       user: username,
//     }));
//     userMessage.value = "";
//   });

//   webSocket.addEventListener("message", (messageJson) => {
//     const message = JSON.parse(messageJson.data);

//     if (messageJson.type === "message") {
//       const messageNode = document.createElement("div");
//       messageNode.appendChild(document.createTextNode(message.Message));
//       messageNode.classList.add("chat-bubble");
//       chatrooms[message.ChatroomName].view.appendChild(messageNode);
//     }
//   })

// }

// const createInviteFormWrapper = document.getElementById("create-invite-form-wrapper");
// const createInviteForm = document.getElementById("create-invite-form");

// createInviteFormWrapper.addEventListener("click", (event) => {
//   if (event.target === createInviteFormWrapper) {
//     createInviteFormWrapper.classList.toggle("visibility");
//   }
// });
// createInviteForm.addEventListener("submit", async (event) => {
//   event.preventDefault();
//   const formData = new FormData(createInviteForm);
//   formData.append("chatroom_name", activeChatroom.textContent);

//   if (!createInviteForm.checkValidity()) {
//     return;
//   }

//   let res = await fetch("/create-invite", {
//     method: "POST",
//     body: formData,
//     mode: "same-origin",
//   });

//   if (res.status !== 202) {
//     console.log("Error creating invite");
//   }
//   const inviteCode = await res.json();
//   console.log(inviteCode);
//   createInviteFormWrapper.classList.toggle("visibility");

//   // const inviteCodeElement = document.createElement("div");
//   // inviteCodeElement.appendChild(document.createTextNode(inviteCode));
//   const inviteCodeWrapper = document.getElementById("invite-code-wrapper");
//   inviteCodeWrapper.classList.toggle("visibility");
//   document.getElementById("invite-code").value = inviteCode;
// });

// const inviteCodeWrapper = document.getElementById("invite-code-wrapper");

// inviteCodeWrapper.addEventListener("click", (event) => {
//   if (event.target === inviteCodeWrapper) {
//     inviteCodeWrapper.classList.toggle("visibility");
//   }
// });

// document.getElementById("back-create-invite").addEventListener("click", (event) => {
//   event.preventDefault();
//   inviteCodeWrapper.classList.toggle("visibility");
//   createInviteFormWrapper.classList.toggle("visibility");
// });

// function addInviteListener(createInvite) {



// }