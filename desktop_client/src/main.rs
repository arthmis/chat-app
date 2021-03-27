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
    widget::{
        Button, Container, Controller, Flex, Label, List, Scope, ScopeTransfer, Split, TextBox,
    },
    AppLauncher, Color, Command, Data, Event, Lens, Point, Selector, SingleUse, Target, Widget,
    WidgetExt, WindowConfig, WindowDesc, WindowLevel, WindowSizePolicy,
};
use futures_util::{SinkExt, StreamExt};
use reqwest::{cookie::Cookie, multipart::Form, redirect::Policy, Client, ClientBuilder, Method};
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

    // login
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
    let res = res?;
    let cookies = res.cookies().collect::<Vec<Cookie>>();
    let stored_cookie = cookies[0].value().to_string();
    // dbg!(cookies);

    // get user's chatrooms
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
    // dbg!(&res?);

    // establish websocket connection
    let (client, res) = task::block_on(async {
        let req = http::request::Builder::new()
            .method(Method::GET)
            .uri("ws://localhost:8000/ws")
            .header("Cookie", format!("{}={}", "session-name", stored_cookie))
            .body(())
            .unwrap();
        let res = connect_async(req).await;
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
    let create = Button::new("Create").on_click(|ctx, data: &mut AppState, env| {
        // dbg!(&data);
        let origin = {
            let window_origin = ctx.window_origin();
            // dbg!(window_origin);
            let size = ctx.window().get_size();
            // dbg!(size);
            Point::new(
                // window_origin.x + size.width / 2.,
                // window_origin.y + size.height / 2.,
                size.width / 2.,
                size.height / 2.,
            )
        };
        let config = WindowConfig::default()
            .set_level(WindowLevel::Modal)
            .show_titlebar(true)
            .resizable(false)
            .window_size_policy(WindowSizePolicy::Content)
            .set_position(origin);
        let subwindow = ctx.new_sub_window(config, create_room(), data.clone(), env.clone());
    });
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
            // dbg!(key_info);
        }
        child.event(ctx, event, data, env)
    }
}

struct AppStateController;

impl Controller<AppState, Container<AppState>> for AppStateController {
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

const CREATE_ROOM: Selector<SingleUse<String>> = Selector::new("app-create-room");
fn create_room() -> impl Widget<AppState> {
    let directions = Label::new("Create your new chatroom");
    let label = Label::new("CHATROOM NAME").with_text_size(12.);
    let textbox = TextBox::new().lens(InviteState::room_name);
    let create = Button::new("Create").on_click(|ctx, data: &mut InviteState, _env| {
        let res = task::block_on(async {
            let mut map = StdMap::new();
            map.insert("chatroom_name", data.room_name.clone());
            let form = Form::new().text("chatroom_name", data.room_name.clone());
            data.client
                .post("http://localhost:8000/create-room")
                // .form(&map)
                .multipart(form)
                .send()
                .await
                .unwrap()
            // .json::<String>()
        });
        if res.status() == 202 {
            dbg!(res.status());
        } else {
            dbg!(res.status());
        }
        ctx.submit_command(Command::new(
            CREATE_ROOM,
            SingleUse::new(data.room_name.clone()),
            Target::Auto,
        ));
        ctx.window().close();
    });
    let layout = Flex::column()
        .with_child(directions)
        .with_spacer(40.)
        .with_child(label)
        .with_child(textbox)
        .with_child(create);
    let layout = layout.fix_height(500.).width(500.).padding(50.);
    Scope::from_function(InviteState::from_app_state, InviteTransfer, layout)
}

#[derive(Debug, Data, Lens, Clone)]
struct InviteState {
    #[data(ignore)]
    client: Arc<Client>,
    room_name: String,
}

impl InviteState {
    fn from_app_state(data: AppState) -> Self {
        Self {
            client: data.http_client,
            room_name: "".to_string(),
        }
    }
}

struct InviteTransfer;

impl ScopeTransfer for InviteTransfer {
    type In = AppState;
    type State = InviteState;

    fn read_input(&self, state: &mut Self::State, inner: &Self::In) {
        // todo!()
    }

    fn write_back_input(&self, state: &Self::State, inner: &mut Self::In) {
        // todo!()
    }
}
