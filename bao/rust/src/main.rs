use std::collections::HashMap;
use std::io::{Cursor, Read, Seek, SeekFrom, Write};
use std::net::SocketAddr;
use std::sync::Arc;

use abao::encode::Encoder;
use portpicker::pick_unused_port;
use tokio::sync::{Mutex, oneshot, RwLock};
use tonic::{Request, Response, Status};
use tonic::transport::Server;
use uuid::Uuid;
use bao::bao_server::Bao;
use tokio::signal::unix::{signal, SignalKind};

use crate::bao::{bao_server, FinishRequest, FinishResponse, HashRequest, HashResponse, NewHasherRequest, NewHasherResponse, VerifyRequest, VerifyResponse};

#[path = "proto/bao.rs"]
mod bao;

struct GlobalState {
    hashers: HashMap<Uuid, Arc<Mutex<Encoder<Cursor<Vec<u8>>>>>>,
}


pub struct BaoService {
    state: Arc<RwLock<GlobalState>>,
}

#[tonic::async_trait]
impl Bao for BaoService {
    async fn new_hasher(&self, _request: Request<NewHasherRequest>) -> Result<Response<NewHasherResponse>, Status> {
        let encoder = Encoder::new_outboard(Cursor::new(Vec::new()));
        let id = Uuid::new_v4();
        {
            let mut state = self.state.write().await;
            state.hashers.insert(id, Arc::new(Mutex::new(encoder)));
        }
        Ok(Response::new(NewHasherResponse {
            id: id.to_string(),
        }))
    }

    async fn hash(&self, request: Request<HashRequest>) -> Result<Response<HashResponse>, Status> {
        let id = Uuid::parse_str(&request.get_ref().id).map_err(|_| Status::invalid_argument("invalid id"))?;
        {
            let state = self.state.read().await;
            let encoder = state.hashers.get(&id).ok_or_else(|| Status::not_found("hasher not found"))?.clone();
            let mut encoder = encoder.lock().await;
            encoder.write_all(&request.get_ref().data).map_err(|_| Status::internal("write failed"))?;
        }
        Ok(Response::new(HashResponse {
            status: true,
        }))
    }

    async fn finish(&self, request: Request<FinishRequest>) -> Result<Response<FinishResponse>, Status> {
        let id = Uuid::parse_str(&request.get_ref().id).map_err(|_| Status::invalid_argument("invalid id"))?;
        let (hash, proof) = {
            let mut state = self.state.write().await;
            let encoder = state.hashers.remove(&id).ok_or_else(|| Status::not_found("hasher not found"))?;
            let encoder = Arc::try_unwrap(encoder).unwrap(); // Unwrap the Arc
            let mut encoder = encoder.lock().await;
            let hash = encoder.finalize()?.as_bytes().to_vec();
            let proof = encoder.inner_mut().get_ref().to_vec();
            (hash, proof)
        };
        Ok(Response::new(FinishResponse {
            hash,
            proof,
        }))
    }

    async fn verify(&self, request: Request<VerifyRequest>) -> Result<Response<VerifyResponse>, Status> {
        let req = request.get_ref();
        let res = verify_internal(
            req.data.clone(),
            req.offset,
            req.proof.clone(),
            from_vec_to_array(req.hash.clone()),
        );

        if res.is_err() {
            Ok(Response::new(VerifyResponse {
                status: false,
            }))
        } else {
            Ok(Response::new(VerifyResponse {
                status: true,
            }))
        }
    }
}

fn verify_internal(
    chunk_bytes: Vec<u8>,
    offset: u64,
    bao_outboard_bytes: Vec<u8>,
    blake3_hash: [u8; 32],
) -> anyhow::Result<u8> {
    let mut slice_stream = abao::encode::SliceExtractor::new_outboard(
        FakeSeeker::new(&chunk_bytes[..]),
        Cursor::new(&bao_outboard_bytes),
        offset,
        262144,
    );

    let mut decode_stream = abao::decode::SliceDecoder::new(
        &mut slice_stream,
        &abao::Hash::from(blake3_hash),
        offset,
        262144,
    );
    let mut decoded = Vec::new();
    decode_stream.read_to_end(&mut decoded)?;

    Ok(1)
}

fn from_vec_to_array<T, const N: usize>(v: Vec<T>) -> [T; N] {
    core::convert::TryInto::try_into(v)
        .unwrap_or_else(|v: Vec<T>| panic!("Expected a Vec of length {} but it was {}", N, v.len()))
}

struct FakeSeeker<R: Read> {
    reader: R,
    bytes_read: u64,
}

impl<R: Read> FakeSeeker<R> {
    fn new(reader: R) -> Self {
        Self {
            reader,
            bytes_read: 0,
        }
    }
}

impl<R: Read> Read for FakeSeeker<R> {
    fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
        let n = self.reader.read(buf)?;
        self.bytes_read += n as u64;
        Ok(n)
    }
}

impl<R: Read> Seek for FakeSeeker<R> {
    fn seek(&mut self, _: SeekFrom) -> std::io::Result<u64> {
        // Do nothing and return the current position.
        Ok(self.bytes_read)
    }
}

impl BaoService {
    fn new(state: Arc<RwLock<GlobalState>>) -> Self {
        BaoService { state }
    }
}


#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let (tx, rx) = oneshot::channel::<()>();
    let health_reporter = tonic_health::server::health_reporter();
    let port = match pick_unused_port() {
        Some(p) => p,
        None => {
            return Err("Failed to pick an unused port".into());
        }
    };

    let addr: SocketAddr = format!("127.0.0.1:{}", port).parse()?;

    println!("1|1|tcp|127.0.0.1:{}|grpc", addr.port());

    let global_state = Arc::new(RwLock::new(GlobalState {
        hashers: HashMap::new(),
    }));

    tokio::spawn(async move {
        let mut term_signal = signal(SignalKind::terminate()).expect("Could not create signal handler");

        // Wait for the terminate signal
        term_signal.recv().await;
        println!("Termination signal received, shutting down server...");

        // Sending a signal through the channel to initiate shutdown.
        // If the receiver is dropped, we don't care about the error.
        let _ = tx.send(());
    });

    Server::builder()
        .max_frame_size((1 << 24) - 1)
        .add_service(bao_server::BaoServer::new(BaoService::new(global_state.clone())))
        .add_service(health_reporter.1)
        .serve_with_shutdown(addr, async {
            // This future completes when the shutdown signal is received,
            // allowing the server to shut down gracefully.
            rx.await.ok();
        })
        .await?;

    Ok(())
}
