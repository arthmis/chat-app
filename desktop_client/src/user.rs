use async_std::task;
use druid::{Data, Lens};
use reqwest::{cookie::Cookie, redirect::Policy, Client, ClientBuilder, Method};
use serde::Deserialize;
use std::collections::HashMap as StdMap;
use tokio::net::TcpStream;
use tokio_tungstenite::{connect_async, MaybeTlsStream, WebSocketStream};

use crate::Room;
use crate::RoomMessage;

#[derive(Deserialize, Default, Debug, Clone, Data, Lens)]
pub struct UserInfo {
    // #[serde(rename = "User")]
    pub name: String,
    // #[serde(rename = "Chatrooms")]
    #[data(ignore)]
    pub chatrooms: Option<Vec<String>>,
    // #[serde(rename = "CurrentRoom")]
    pub current_room: String,
}

pub fn http_client() -> reqwest::Result<Client> {
    ClientBuilder::new()
        .cookie_store(true)
        .gzip(true)
        .redirect(Policy::none())
        .build()
}
#[derive(Debug, Clone)]
pub struct LoginInfo {
    pub email: String,
    pub password: String,
}

pub fn client_login(client: &mut Client, login_info: LoginInfo) -> Result<String, anyhow::Error> {
    let res = task::block_on(async {
        let mut map = StdMap::new();
        map.insert("email", &login_info.email);
        map.insert("password", &login_info.password);
        let res = client
            .post("http://localhost:8000/api/user/login")
            .form(&map)
            .send()
            .await;
        res
    });
    let res = res?;
    if res.status() == 200 {
        anyhow::bail!(
            "Login was not successful. Status code was: {}",
            res.status()
        );
    }
    let cookies = res.cookies().collect::<Vec<Cookie>>();
    let stored_cookie = cookies[0].value().to_string();
    Ok(stored_cookie)
}

// get user's chatrooms and information
pub fn get_user_chatrooms_info(
    client: &mut Client,
) -> Result<(String, Vec<Room>, usize, UserInfo), anyhow::Error> {
    let res = task::block_on(async {
        // let mut map = StdMap::new();
        // map.insert("email", "kupa@gmail.com");
        // map.insert("password", "secretpassy");
        client
            .post("http://localhost:8000/api/user/chatrooms")
            // .form(&map)
            .send()
            .await?
            .json::<UserInfo>()
            .await
    });
    let res = res?;
    let user_info = res.clone();
    let rooms = res.chatrooms.unwrap_or_default();
    let rooms = rooms
        .into_iter()
        .enumerate()
        .map(|(idx, name)| Room { name, idx })
        .collect::<Vec<Room>>();

    let selected_room = res.current_room;
    let selected_room_idx =
        { rooms.iter().position(|room| room.name == selected_room) }.unwrap_or(0);
    Ok((selected_room, rooms, selected_room_idx, user_info))
}

// get user's current room messages
pub fn user_current_room_messages(
    client: &mut Client,
    selected_room: &str,
) -> Result<Vec<RoomMessage>, anyhow::Error> {
    let res = task::block_on(async {
        let mut map = StdMap::new();
        map.insert("chatroom_name", selected_room);
        // map.insert("password", "secretpassy");
        let res = client
            .post("http://localhost:8000/api/room/messages")
            .form(&map)
            .send()
            .await?
            .json::<Vec<RoomMessage>>()
            .await;
        res
    });
    dbg!(&res);
    Ok(res?.into_iter().rev().collect::<Vec<RoomMessage>>())
}

// establish websocket connection
pub fn establish_ws_conn(
    stored_cookie: &str,
) -> Result<WebSocketStream<MaybeTlsStream<TcpStream>>, anyhow::Error> {
    let res = task::block_on(async {
        let req = http::request::Builder::new()
            .method(Method::GET)
            .uri("ws://localhost:8000/api/ws")
            .header("Cookie", format!("{}={}", "session-name", stored_cookie))
            .body(())?;
        connect_async(req).await
    });
    let (stream, _res) = res?;
    // dbg!(&res);
    Ok(stream)
}
