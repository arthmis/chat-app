use std::{
    collections::HashMap as StdMap,
    error::Error,
    fmt::Debug,
    sync::{mpsc::Sender, Arc},
    thread,
};

use async_std::task;
use druid::{
    im::HashMap,
    widget::{Button, Container, Controller, Flex, Label, List, Split, TextBox},
    AppLauncher, Color, Data, Event, Lens, Widget, WidgetExt, WindowDesc,
};
use futures_util::{SinkExt, StreamExt};
use reqwest::{redirect::Policy, Client, ClientBuilder};
use serde::{Deserialize, Serialize};
use tokio::net::TcpStream;
use tokio_tungstenite::{connect_async, tungstenite::Message, MaybeTlsStream, WebSocketStream};

fn main() -> Result<(), Box<dyn Error + Send + Sync>> {
    let client = http_client().unwrap();
    // signs up to chat app
    // let (client, res) = async_std::task::block_on(async {
    //     map.insert("email", "kupa@gmail.com");
    //     map.insert("username", "art");
    //     map.insert("password", "secretpassy");
    //     map.insert("confirmPassword", "secretpassy");
    //     let res = client
    //         .post("http://localhost:8000/signup")
    //         .form(&map)
    //         .send()
    //         .await;
    //     (client, res)
    // });
    // dbg!(res?);
    let (client, res) = task::block_on(async {
        let mut map = StdMap::new();
        map.insert("email", "kupa@gmail.com");
        map.insert("password", "secretpassy");
        let res = client
            .post("http://localhost:8000/login")
            .form(&map)
            .send()
            .await;
        (client, res)
    });
    // dbg!(res?);
    res?;
    let (client, res) = task::block_on(async {
        // let mut map = StdMap::new();
        // map.insert("email", "kupa@gmail.com");
        // map.insert("password", "secretpassy");
        let res = client
            .post("http://localhost:8000/user/chatrooms")
            // .form(&map)
            .send()
            .await
            .unwrap()
            .json::<ChatroomInfo>()
            .await;
        (client, res)
    });
    dbg!(&res?);
    let (client, res) = task::block_on(async {
        // let mut map = StdMap::new();
        // map.insert("email", "kupa@gmail.com");
        // map.insert("password", "secretpassy");
        let res = connect_async("ws://localhost:8000/ws").await;
        // let client = client_async(
        //     "http://localhost:8000/ws",
        //     stream.get_mut(), // BufWriter::new(Vec::new()),
        // )
        // .await;
        // let res = client
        //     .get("http://localhost:8000/ws")
        //     .send()
        //     .await;
        // .unwrap()
        // .json::<ChatroomInfo>()
        // .await;
        (client, res)
    });
    let (stream, res) = res.unwrap();
    dbg!(&res);
    // res?;
    let window = WindowDesc::new(ui()).title("Rume");
    let app_state = AppState::new(HashMap::new(), Arc::new(Vec::new()), client, stream);
    AppLauncher::with_window(window)
        .log_to_console()
        .launch(app_state)?;

    Ok(())
}

#[derive(Deserialize, Debug, Clone)]
struct ChatroomInfo {
    chatrooms: Option<Vec<String>>,
    current_room: String,
}

fn http_client() -> reqwest::Result<Client> {
    ClientBuilder::new()
        .cookie_store(true)
        .gzip(true)
        .redirect(Policy::none())
        .build()
}

#[derive(Data, Lens, Clone)]
struct AppState {
    chatrooms: HashMap<String, Arc<Vec<String>>>,
    rooms: Arc<Vec<String>>,
    selected_room: usize,
    http_client: Arc<Client>,
    // ws: Arc<WebSocketStream<MaybeTlsStream<TcpStream>>>,
    #[data(ignore)]
    channel: Sender<String>,
    textbox: String,
}

impl AppState {
    fn new(
        chatrooms: HashMap<String, Arc<Vec<String>>>,
        rooms: Arc<Vec<String>>,
        http_client: Client,
        ws: WebSocketStream<MaybeTlsStream<TcpStream>>,
    ) -> Self {
        let (tx, rx) = std::sync::mpsc::channel();

        thread::spawn(move || {
            task::block_on(async {
                // let stream = ws.get_mut();
                let (mut write, read) = ws.split();
                loop {
                    match rx.recv() {
                        Ok(bytes) => {
                            dbg!(&bytes);
                            let message = {
                                let message = ChatMessage {
                                    user: "Art".to_string(),
                                    room: "test room".to_string(),
                                    message: bytes,
                                };
                                let message = serde_json::to_string(&message);
                                // let bytes = bytes.as_bytes();

                                Message::Text(message.unwrap())
                            };
                            write.send(message).await.unwrap();
                        }
                        Err(err) => println!("{}", err),
                    }
                }
            });
        });

        Self {
            chatrooms,
            rooms,
            selected_room: 0,
            http_client: Arc::new(http_client),
            channel: tx,
            textbox: String::new(),
        }
    }
}

impl Debug for AppState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("AppState")
            .field("chatrooms", &self.chatrooms)
            .field("rooms", &self.rooms)
            .field("selected_room", &self.selected_room)
            .finish()
    }
}

struct ChatroomsLens;
impl Lens<AppState, Arc<Vec<String>>> for ChatroomsLens {
    fn with<V, F: FnOnce(&Arc<Vec<String>>) -> V>(&self, data: &AppState, f: F) -> V {
        if !data.rooms.is_empty() {
            match data.chatrooms.get(&data.rooms[data.selected_room]) {
                Some(room) => f(room),
                None => f(&Arc::new(Vec::new())),
            }
        } else {
            f(&Arc::new(Vec::new()))
        }
    }

    fn with_mut<V, F: FnOnce(&mut Arc<Vec<String>>) -> V>(&self, data: &mut AppState, f: F) -> V {
        if !data.rooms.is_empty() {
            match data.chatrooms.get_mut(&data.rooms[data.selected_room]) {
                Some(room) => f(room),
                None => f(&mut Arc::new(Vec::new())),
            }
        } else {
            f(&mut Arc::new(Vec::new()))
        }
    }
}

#[derive(Debug, Clone, Deserialize, Serialize, Data, Lens)]
struct ChatMessage {
    #[serde(rename = "User")]
    user: String,
    #[serde(rename = "ChatroomName")]
    room: String,
    #[serde(rename = "Message")]
    message: String,
}

fn ui() -> impl Widget<AppState> {
    let rooms =
        List::new(|| Label::dynamic(|room_name: &String, _env| room_name.to_string()).padding(5.0))
            .lens(AppState::rooms);
    let invite = Button::new("Invite");
    let create = Button::new("Create");
    let buttons = Flex::row().with_child(invite).with_child(create).center();
    let left = Flex::column()
        .with_flex_child(rooms, 3.0)
        .with_flex_child(buttons, 1.0)
        .expand_height();

    let textbox = TextBox::new()
        .controller(TextboxController)
        .lens(AppState::textbox);
    let messages =
        List::new(|| Label::dynamic(|room_name: &String, _env| room_name.to_string()).padding(5.0))
            .lens(ChatroomsLens);
    let right = Flex::column()
        .with_flex_child(messages, 3.0)
        .with_flex_child(textbox, 1.0);

    Split::columns(left, right)
        .solid_bar(true)
        .bar_size(3.0)
        .split_point(0.2)
        .background(Color::WHITE)
        .controller(AppStateController)
}

struct TextboxController;
impl Controller<String, TextBox<String>> for TextboxController {
    fn event(
        &mut self,
        child: &mut TextBox<String>,
        ctx: &mut druid::EventCtx,
        event: &druid::Event,
        data: &mut String,
        env: &druid::Env,
    ) {
        if let Event::KeyDown(key_info) = event {
            dbg!(key_info);
        }
        child.event(ctx, event, data, env)
    }

    // fn lifecycle(
    //     &mut self,
    //     child: &mut TextBox<String>,
    //     ctx: &mut druid::LifeCycleCtx,
    //     event: &druid::LifeCycle,
    //     data: &String,
    //     env: &druid::Env,
    // ) {
    //     child.lifecycle(ctx, event, data, env)
    // }

    // fn update(&mut self, child: &mut TextBox<String>, ctx: &mut druid::UpdateCtx, old_data: &String, data: &String, env: &druid::Env) {
    //     child.update(ctx, old_data, data, env)
    // }
}

struct AppStateController;

impl Controller<AppState, Container<AppState>> for AppStateController {
    // fn event(
    //     &mut self,
    //     child: &mut Container<AppState>,
    //     ctx: &mut druid::EventCtx,
    //     event: &Event,
    //     data: &mut AppState,
    //     env: &druid::Env,
    // ) {
    //     child.event(ctx, event, data, env)
    // }

    // fn lifecycle(
    //     &mut self,
    //     child: &mut Container<AppState>,
    //     ctx: &mut druid::LifeCycleCtx,
    //     event: &druid::LifeCycle,
    //     data: &AppState,
    //     env: &druid::Env,
    // ) {
    //     child.lifecycle(ctx, event, data, env)
    // }

    fn update(
        &mut self,
        child: &mut Container<AppState>,
        ctx: &mut druid::UpdateCtx,
        old_data: &AppState,
        data: &AppState,
        env: &druid::Env,
    ) {
        if !data.textbox.same(&old_data.textbox) {
            match data.channel.send(data.textbox.to_string()) {
                Ok(_) => (),
                Err(err) => {
                    println!("{}", err);
                }
            }
        }
        child.update(ctx, old_data, data, env)
    }
}
