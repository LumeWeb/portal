#![feature(async_fn_in_trait)]
#![allow(incomplete_features)]

use io::Read;
use std::collections::hash_map::Entry;
use std::collections::HashMap;
use std::fs::{File};
use std::io;
use std::io::{Cursor, Write};
use std::sync::{Arc};

use abao::encode::Encoder;
use async_trait::async_trait;
use atomic_counter::{AtomicCounter, ConsistentCounter};
use parking_lot::Mutex;
use tonic::{Request, Response, Status};
use tonic::transport::Server;
use tonic_health::server::HealthReporter;

use crate::proto::bao::bao_server::{Bao, BaoServer};
use crate::proto::bao::WriteRequest;
use crate::proto::google::protobuf::{BytesValue, Empty, StringValue, UInt32Value};
use crate::unique_port::UniquePort;

mod proto;
mod unique_port;
mod grpc;

async fn driver_service_status(mut reporter: HealthReporter) {
    reporter.set_serving::<BaoServer<BaoService>>().await;
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut uport = UniquePort::default();
    let port = uport.get_unused_port().expect("No ports free");
    println!("{}", format!("1|1|tcp|127.0.0.1:{}|grpc", port));

    let (mut health_reporter, health_service) = tonic_health::server::health_reporter();

    health_reporter.set_serving::<BaoServer<BaoService>>().await;

    tokio::spawn(driver_service_status(health_reporter.clone()));

    let addr = format!("127.0.0.1:{}", port).parse().unwrap();
    let bao_service = BaoService::default();
    let server = BaoServer::new(bao_service);
    Server::builder()
        .add_service(health_service)
        .add_service(server)
        .add_service(grpc::grpc_stdio::new_server())
        .serve(addr)
        .await?;

    Ok(())
}

#[derive(Debug, Default)]
pub struct BaoService {
    requests:  Arc<Mutex<HashMap<u32, Encoder<Cursor<Vec<u8>>>>>>,
    counter: ConsistentCounter,
}

#[async_trait]
impl Bao for BaoService {
    async fn init(&self, _request: Request<Empty>) -> Result<Response<UInt32Value>, Status> {
        let next_id = self.counter.inc() as u32;
        let tree = Vec::new();
        let cursor = Cursor::new(tree);
        let encoder = Encoder::new_outboard(cursor);

        let mut req = self.requests.lock();
        req.insert(next_id, encoder);

        Ok(Response::new(UInt32Value { value: next_id }))
    }

    async fn write(&self, request: Request<WriteRequest>) -> Result<Response<Empty>, Status> {
        let r = request.into_inner();
        let mut req = self.requests.lock();
        if let Some(encoder) = req.get_mut(&r.id) {
            encoder.write(&r.data)?;
        } else {
            return Err(Status::invalid_argument("invalid id"));
        }

        Ok(Response::new(Empty::default()))
    }

    async fn finalize(
        &self,
        request: Request<UInt32Value>,
    ) -> Result<Response<BytesValue>, Status> {
        let r = request.into_inner();
        let mut req = self.requests.lock();
        match req.entry(r.value) {
            Entry::Occupied(mut entry) => {
                let encoder = entry.get_mut();
                let ret = encoder.finalize().unwrap();
                let bytes = ret.as_bytes().to_vec();
                Ok(Response::new(BytesValue { value: bytes }))
            }
            Entry::Vacant(_) => {
                Err(Status::invalid_argument("invalid id"))
            }
        }
    }


    async fn destroy(&self, request: Request<UInt32Value>) -> Result<Response<Empty>, Status> {
        let r = request.into_inner();
        let mut req = self.requests.lock();
        if req.remove(&r.value).is_none() {
            return Err(Status::invalid_argument("invalid id"));
        }

        Ok(Response::new(Empty::default()))
    }

    async fn compute_file(&self, request: Request<StringValue>) -> Result<Response<BytesValue>, Status> {
        let r = request.into_inner();
        let tree = Vec::new();
        let cursor = Cursor::new(tree);
        let mut encoder = Encoder::new_outboard(cursor);
        let mut input =  File::open(r.value)?;

        copy_reader_to_writer(&mut input, &mut encoder)?;

        let ret = encoder.finalize().unwrap();
        let bytes = ret.as_bytes().to_vec();
        Ok(Response::new(BytesValue { value: bytes }))
    }
}
fn copy_reader_to_writer(
    reader: &mut impl Read,
    writer: &mut impl Write,
) -> io::Result<u64> {
    // At least 16 KiB is necessary to use AVX-512 with BLAKE3.
    let mut buf = [0; 65536];
    let mut written = 0;
    loop {
        let len = match reader.read(&mut buf) {
            Ok(0) => return Ok(written),
            Ok(len) => len,
            Err(ref e) if e.kind() == io::ErrorKind::Interrupted => continue,
            Err(e) => return Err(e),
        };
        writer.write_all(&buf[..len])?;
        written += len as u64;
    }
}
