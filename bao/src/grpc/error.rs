use hyper::http::uri::InvalidUri;
use thiserror::Error as ThisError;
use tokio::sync::mpsc::error::SendError;
use tonic::transport::Error as TonicError;
use std::fmt::{Debug};

pub fn into_status(err: Error) -> tonic::Status {
    tonic::Status::unknown(format!("{}", err))
}

#[derive(Debug, ThisError)]
pub enum Error {
    #[error("Error with IO: {0}")]
    Io(#[from] std::io::Error),
    #[error("Error with tonic (gRPC) transport: {0}")]
    TonicTransport(#[from] TonicError),
    #[error("Error parsing string into a network address: {0}")]
    AddrParser(#[from] std::net::AddrParseError),
    #[error("Error sending on a mpsc channel: {0}")]
    Send(String),
    #[error("Invalid Uri: {0}")]
    InvalidUri(#[from] InvalidUri),
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

impl<T> From<SendError<T>> for Error {
    fn from(_err: SendError<T>) -> Self {
        Self::Send(format!(
            "unable to send {} on a mpsc channel",
            std::any::type_name::<T>()
        ))
    }
}
