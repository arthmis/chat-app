use std::{
    collections::HashMap as StdMap,
    error::Error,
    fmt::Debug,
    mem,
    sync::{mpsc::Sender, Arc},
    thread,
};

use async_std::task;
use druid::{
    im::HashMap,
    theme::{self, TEXTBOX_INSETS},
    widget::{
        Container, Controller, CrossAxisAlignment, Flex, Label, List, ListIter, MainAxisAlignment,
        Painter, Scope, ScopeTransfer, Split, TextBox,
    },
    AppLauncher, Code, Color, Command, Data, Event, EventCtx, ExtEventSink, Insets, Lens, Point,
    RenderContext, Selector, SingleUse, Target, Widget, WidgetExt, WindowConfig, WindowDesc,
    WindowLevel, WindowSizePolicy,
};
use futures_util::{SinkExt, StreamExt};
use reqwest::{multipart::Form, Client};
use serde::{Deserialize, Serialize};
use tokio::net::TcpStream;
use tokio_tungstenite::{tungstenite::Message, MaybeTlsStream, WebSocketStream};
use widgets::button::{
    CHAT_BUTTON_ACTIVE, CHAT_BUTTON_ACTIVE_BORDER, CHAT_BUTTON_BORDER, CHAT_BUTTON_COLOR,
    CHAT_BUTTON_HOVER, CHAT_BUTTON_HOVER_BORDER,
};

mod user;
mod widgets;

use crate::user::*;
use crate::widgets::button::Button;

fn main() -> Result<(), Box<dyn Error + Send + Sync>> {
    let window = WindowDesc::new(login())
        .title("Rume")
        .window_size_policy(WindowSizePolicy::Content);
    let app = AppLauncher::with_window(window).log_to_console();

    let app_state = AppState::default();
    app.configure_env(|env, _data| {
        env.set(CHAT_BUTTON_ACTIVE, Color::from_hex_str("#b68d40").unwrap());
        env.set(CHAT_BUTTON_HOVER, Color::from_hex_str("#f4ebd0").unwrap());
        env.set(CHAT_BUTTON_COLOR, Color::from_hex_str("#d6ad60").unwrap());
        env.set(CHAT_BUTTON_ACTIVE_BORDER, Color::WHITE);
        env.set(CHAT_BUTTON_HOVER_BORDER, Color::WHITE);
        env.set(CHAT_BUTTON_BORDER, Color::WHITE);

        // for textbox styling
        env.set(
            theme::BACKGROUND_LIGHT,
            Color::from_hex_str("#dddddd").unwrap(),
        );
        env.set(theme::BORDER_DARK, Color::from_hex_str("#dddddd").unwrap());
        env.set(
            theme::PRIMARY_LIGHT,
            Color::from_hex_str("#122620").unwrap(),
        );
        env.set(TEXTBOX_INSETS, Insets::uniform(10.0))
    })
    .launch(app_state)?;

    Ok(())
}

impl Default for AppState {
    fn default() -> Self {
        let (tx, _) = std::sync::mpsc::channel();
        AppState {
            chatrooms: HashMap::new(),
            selected_room: None,
            rooms: Arc::new(Vec::new()),
            http_client: Arc::new(Client::new()),
            channel: tx,
            textbox: String::new(),
            user: UserInfo::default(),
        }
    }
}

#[derive(Data, Lens, Clone)]
struct AppState {
    chatrooms: HashMap<String, Arc<Vec<String>>>,
    rooms: Arc<Vec<Room>>,
    selected_room: Option<usize>,
    http_client: Arc<Client>,
    #[data(ignore)]
    channel: Sender<ChatMessage>,
    textbox: String,
    user: UserInfo,
}

#[derive(Data, Lens, Clone, Debug, Eq, PartialEq, Hash)]
pub struct Room {
    name: String,
    idx: usize,
}

// struct User {
//     name: String,
//     chatrooms: std::collections::HashSet<String>,
// }

impl AppState {
    fn new(
        chatrooms: HashMap<String, Arc<Vec<String>>>,
        rooms: Arc<Vec<Room>>,
        selected_room: usize,
        http_client: Client,
        ws: WebSocketStream<MaybeTlsStream<TcpStream>>,
        event_sink: ExtEventSink,
        user_info: UserInfo,
    ) -> Self {
        let (tx, rx) = std::sync::mpsc::channel();
        let (mut write, mut read) = ws.split();

        // spawns thread to write messages
        thread::spawn(move || {
            task::block_on(async {
                loop {
                    match rx.recv() {
                        Ok(message) => {
                            let message = Message::Text(serde_json::to_string(&message).unwrap());
                            write.send(message).await.unwrap();
                        }
                        Err(err) => println!("{}", err),
                    }
                }
            });
        });

        // spawns thread to read messages
        thread::spawn(move || {
            task::block_on(async {
                loop {
                    if let Some(res) = read.next().await {
                        let message = res.unwrap();
                        let message: ChatMessage =
                            serde_json::from_str(message.to_text().unwrap()).unwrap();
                        dbg!(&message);
                        event_sink
                            .submit_command(RECEIVE_MESSAGE, SingleUse::new(message), Target::Auto)
                            .unwrap();
                    }
                }
            });
        });

        Self {
            chatrooms,
            rooms,
            selected_room: Some(selected_room),
            http_client: Arc::new(http_client),
            channel: tx,
            textbox: String::new(),
            user: user_info,
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
            match data
                .chatrooms
                .get(&data.rooms[data.selected_room.unwrap()].name)
            {
                Some(room) => f(room),
                None => f(&Arc::new(Vec::new())),
            }
        } else {
            f(&Arc::new(Vec::new()))
        }
    }

    fn with_mut<V, F: FnOnce(&mut Arc<Vec<String>>) -> V>(&self, data: &mut AppState, f: F) -> V {
        if !data.rooms.is_empty() {
            match data
                .chatrooms
                .get_mut(&data.rooms[data.selected_room.unwrap()].name)
            {
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

fn login() -> impl Widget<AppState> {
    let textbox_color = Color::BLACK;
    let textbox_font_size = 21.;

    let email_label: Label<LoginState> = Label::new("Email")
        .with_text_size(14.)
        .with_text_color(Color::BLACK);
    let email_textbox = TextBox::new()
        .with_text_size(textbox_font_size)
        .with_text_color(textbox_color.clone())
        .controller(FormController)
        .fix_width(200.)
        .lens(LoginState::email);
    let email = Flex::column()
        .with_child(email_label)
        .with_spacer(5.)
        .with_child(email_textbox)
        .cross_axis_alignment(CrossAxisAlignment::Start);

    let password_label = Label::new("Password")
        .with_text_size(14.)
        .with_text_color(Color::BLACK);
    let password_textbox = TextBox::new()
        .with_text_size(textbox_font_size)
        .with_text_color(textbox_color)
        .controller(FormController)
        .fix_width(200.)
        .lens(LoginState::password);
    let password = Flex::column()
        .with_child(password_label)
        .with_spacer(5.)
        .with_child(password_textbox)
        .cross_axis_alignment(CrossAxisAlignment::Start);

    let button = Button::new("Submit")
        .on_click(|ctx, _: &mut LoginState, _| {
            ctx.submit_command(Command::new(ATTEMPT_LOGIN, (), Target::Auto))
        })
        .fix_width(100.)
        .height(30.);

    let layout = Flex::column()
        .with_child(email)
        .with_spacer(10.)
        .with_child(password)
        .with_spacer(10.)
        .with_child(button);

    let login = Container::new(layout)
        .padding(20.)
        .background(Color::WHITE)
        .controller(LoginController)
        .fix_size(400., 200.);

    Scope::from_function(|_| LoginState::default(), LoginStateTransfer, login)
}

#[derive(Data, Default, Debug, Clone, Lens)]
struct LoginState {
    email: String,
    password: String,
    login_success: Option<AppState>,
}

struct FormController;

impl Controller<String, TextBox<String>> for FormController {
    fn event(
        &mut self,
        child: &mut TextBox<String>,
        ctx: &mut EventCtx,
        event: &Event,
        data: &mut String,
        env: &druid::Env,
    ) {
        if let Event::KeyUp(key) = event {
            if key.code == Code::Enter {
                ctx.submit_command(Command::new(ATTEMPT_LOGIN, (), Target::Auto))
            }
            ctx.set_handled();
        }
        child.event(ctx, event, data, env)
    }
}
struct LoginController;
impl Controller<LoginState, Container<LoginState>> for LoginController {
    fn event(
        &mut self,
        child: &mut Container<LoginState>,
        ctx: &mut druid::EventCtx,
        event: &Event,
        data: &mut LoginState,
        env: &druid::Env,
    ) {
        match event {
            Event::Command(selector) if selector.is(ATTEMPT_LOGIN) => {
                let login_info = LoginInfo {
                    email: data.email.clone(),
                    password: data.password.clone(),
                };
                let mut client = http_client().unwrap();
                match client_login(&mut client, login_info) {
                    Ok(cookie) => match get_user_chatrooms_info(&mut client) {
                        Ok((selected_room, rooms, selected_room_idx, user_info)) => {
                            match user_current_room_messages(&mut client, &selected_room) {
                                Ok(messages) => match establish_ws_conn(&cookie) {
                                    Ok(stream) => {
                                        let mut map = HashMap::new();
                                        map.insert(selected_room, Arc::new(messages));
                                        let app_state = AppState::new(
                                            map,
                                            Arc::new(rooms),
                                            selected_room_idx,
                                            client,
                                            stream,
                                            ctx.get_external_handle(),
                                            user_info,
                                        );
                                        data.login_success = Some(app_state);
                                        ctx.window().close();
                                        let window = WindowDesc::new(ui()).title("Rume");
                                        ctx.new_window(window);
                                        ctx.set_handled();
                                        return;
                                    }
                                    Err(err) => println!("{}", err),
                                },
                                Err(err) => {
                                    println!("{}", err)
                                }
                            }
                        }
                        Err(err) => println!("{}", err),
                    },
                    Err(err) => println!("{}", err),
                }
            }
            _ => (),
        }
        child.event(ctx, event, data, env)
    }
}
// const UPDATE_AFTER_LOGIN: Selector<SingleUse<AppState>> = Selector::new("UPDATE_AFTER_LOGIN");
const ATTEMPT_LOGIN: Selector<()> = Selector::new("ATTEMPT_LOGIN");
fn ui() -> impl Widget<AppState> {
    let rooms = List::new(|| {
        // Label::dynamic(|(room, selected_room), _env| room.name.to_string())
        Label::dynamic(|data: &(Room, usize), _env| data.0.name.to_string())
            .with_text_color(Color::BLACK)
            .center()
            .expand_width()
            .padding(5.0)
            .background(Painter::new(|ctx, data: &(Room, usize), _env| {
                let is_hot = ctx.is_hot();
                let is_active = ctx.is_active();
                let (room, selected) = data;
                let is_selected = room.idx == *selected;

                let background_color = if is_active {
                    Color::GREEN
                } else if is_hot {
                    Color::BLUE
                } else if is_selected {
                    Color::GRAY
                } else {
                    Color::WHITE
                };

                let rect = ctx.size().to_rect();
                ctx.stroke(rect, &background_color, 1.);
                ctx.fill(rect, &background_color);
            }))
            .on_click(|ctx, data, _env| {
                let (room, _selected) = data;
                ctx.submit_command(Command::new(
                    CHANGING_ROOM,
                    (room.idx, data.0.name.clone()),
                    Target::Auto,
                ));
            })
    })
    .scroll()
    .vertical();
    // .lens(AppState::rooms);
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
        let _subwindow = ctx.new_sub_window(config, create_room(), data.clone(), env.clone());
    });
    let buttons = Flex::row().with_child(invite).with_child(create).center();
    let left = Flex::column()
        .with_flex_child(rooms.scroll().vertical(), 9.0)
        .with_flex_child(buttons, 1.0)
        .main_axis_alignment(MainAxisAlignment::SpaceBetween)
        .expand_height();

    let textbox = TextBox::new()
        .controller(TextboxController)
        .lens(AppState::textbox)
        .fix_height(25.)
        .width(300.);
    let send_message = Button::new("Send").on_click(|ctx, data: &mut AppState, _env| {
        ctx.submit_command(Command::new(
            SEND_MESSAGE,
            SingleUse::new(data.textbox.clone()),
            Target::Auto,
        ));
    });
    let message_box = Flex::row()
        .with_child(textbox)
        .with_child(send_message)
        .main_axis_alignment(MainAxisAlignment::Center)
        .expand_width()
        .height(40.)
        .center();

    let messages = List::new(|| {
        // let user = Label::dynamic(|room_name: &ChatMessage, _env| room_name.to_string())
        //     .with_text_color(Color::BLACK)
        //     .padding(5.0);

        // let message = Label::dynamic(|room_name: &ChatMessage, _env| room_name.to_string())
        //     .with_text_color(Color::BLACK)
        //     .padding(5.0);

        // Flex::column().with_child(user).with_child(message)
        Label::dynamic(|room_name: &String, _env| room_name.to_string())
            .with_text_color(Color::BLACK)
            .padding(5.0)
    })
    .lens(ChatroomsLens)
    .scroll()
    .vertical()
    .expand()
    .padding(5.0);
    let right = Flex::column()
        .with_flex_child(messages, 9.0)
        // .with_flex_child(message_box, 1.0);
        .with_child(message_box);

    Split::columns(left, right)
        .solid_bar(true)
        .bar_size(3.0)
        .split_point(0.2)
        .background(Color::WHITE)
        .controller(AppStateController)
}
const SEND_MESSAGE: Selector<SingleUse<String>> = Selector::new("app-send-message");
const RECEIVE_MESSAGE: Selector<SingleUse<ChatMessage>> = Selector::new("app-receive-message");

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
            if let Code::Enter = key_info.code {
                ctx.submit_command(Command::new(
                    SEND_MESSAGE,
                    SingleUse::new(data.clone()),
                    Target::Auto,
                ));
                data.clear();
                // don't want textbox to handle any events
                // because Enter was pressed and message was sent
                return;
            }
        }
        child.event(ctx, event, data, env)
    }
}

struct AppStateController;

impl Controller<AppState, Container<AppState>> for AppStateController {
    fn event(
        &mut self,
        child: &mut Container<AppState>,
        ctx: &mut druid::EventCtx,
        event: &Event,
        data: &mut AppState,
        env: &druid::Env,
    ) {
        match event {
            Event::Command(selector) if selector.is(SEND_MESSAGE) => {
                let message = selector.get_unchecked(SEND_MESSAGE).take().unwrap();
                let message = ChatMessage {
                    user: data.user.name.to_owned(),
                    room: data.rooms[data.selected_room.unwrap()].name.clone(),
                    message,
                };

                match data.channel.send(message) {
                    Ok(_) => (),
                    Err(err) => {
                        println!("{}", err);
                    }
                }

                data.textbox.clear();
            }
            Event::Command(selector) if selector.is(RECEIVE_MESSAGE) => {
                let message = selector.get_unchecked(RECEIVE_MESSAGE).take().unwrap();
                // dbg!(&data.chatrooms);
                let messages = Arc::make_mut(&mut data.chatrooms[&message.room]);
                messages.push(message.message);
                data.chatrooms[&message.room] = Arc::new(messages.to_owned());
            }
            Event::Command(selector) if selector.is(CHANGING_ROOM) => {
                let (new_selected, room_name) = selector.get_unchecked(CHANGING_ROOM);
                // get room's messages
                // let (client, res) = task::block_on(async {
                if !data.chatrooms.contains_key(room_name) {
                    let res = task::block_on(async {
                        let mut map = StdMap::new();
                        let room = room_name.to_owned();
                        map.insert("chatroom_name", &room);
                        // map.insert("password", "secretpassy");
                        let client = &data.http_client;
                        client
                            .post("http://localhost:8000/api/room/messages")
                            .form(&map)
                            .send()
                            .await
                            .unwrap()
                            .json::<Vec<String>>()
                            .await
                    });
                    // dbg!(&res);
                    let messages = res.unwrap().into_iter().rev().collect::<Vec<String>>();
                    let _room_messages = data
                        .chatrooms
                        .insert(room_name.to_owned(), Arc::new(messages));
                }
                data.selected_room = Some(*new_selected);
            }
            Event::Command(selector) if selector.is(CREATE_ROOM) => {
                let new_room = selector.get_unchecked(CREATE_ROOM).take().unwrap();

                data.chatrooms
                    .insert(new_room.clone(), Arc::new(Vec::new()));

                data.selected_room = Some(data.rooms.len());

                let new_rooms = Arc::make_mut(&mut data.rooms);
                new_rooms.push(Room {
                    name: new_room,
                    idx: new_rooms.len(),
                });
                data.rooms = Arc::new(new_rooms.to_owned());

                ctx.request_paint();
            }
            _ => (),
        }
        child.event(ctx, event, data, env)
    }
}

const CHANGING_ROOM: Selector<(usize, String)> = Selector::new("app-change-room");
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
                .post("http://localhost:8000/api/room/create")
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
            // change this to specifically target main window
            // so I will need to store parent window's id
            Target::Global,
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

    fn read_input(&self, _state: &mut Self::State, _inner: &Self::In) {
        // todo!()
    }

    fn write_back_input(&self, _state: &Self::State, _inner: &mut Self::In) {
        // todo!()
    }
}

impl ListIter<(Room, usize)> for AppState {
    fn for_each(&self, mut cb: impl FnMut(&(Room, usize), usize)) {
        for (i, item) in self.rooms.iter().enumerate() {
            let d = (item.to_owned(), self.selected_room.unwrap());
            cb(&d, i);
        }
    }

    fn for_each_mut(&mut self, mut cb: impl FnMut(&mut (Room, usize), usize)) {
        let mut new_data = Vec::with_capacity(self.data_len());
        let mut any_changed = false;
        let mut new_selected = self.selected_room;

        for (i, item) in self.rooms.iter().enumerate() {
            let mut d = (item.to_owned(), self.selected_room.unwrap());
            cb(&mut d, i);

            // if !any_changed && !(*item, i, self.selected_room).same(&d) {
            if !any_changed && !self.selected_room.unwrap().same(&d.1) {
                any_changed = true;
                new_selected = Some(d.1);
            }
            // dbg!(any_changed);
            new_data.push(d.0);
        }

        if any_changed {
            self.rooms = Arc::new(new_data);
            self.selected_room = new_selected;
        }
    }

    fn data_len(&self) -> usize {
        self.rooms.len()
    }
}

struct LoginStateTransfer;

impl ScopeTransfer for LoginStateTransfer {
    type In = AppState;

    type State = LoginState;

    fn read_input(&self, _state: &mut Self::State, _inner: &Self::In) {}

    fn write_back_input(&self, state: &Self::State, inner: &mut Self::In) {
        if let Some(app_state) = state.login_success.clone() {
            let _ = mem::replace(inner, app_state);
        }
    }
}
