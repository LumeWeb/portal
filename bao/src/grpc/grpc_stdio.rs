// Copied from: https://github.com/hashicorp/go-plugin/blob/master/grpc_controller.go
use anyhow::{Context, Result};
use async_stream::stream;
use futures::stream::Stream;
use gag::BufferRedirect;
use std::io::Read;
use std::pin::Pin;
use tokio::time::{sleep, Duration};
use tokio_stream::StreamExt;
use tonic::{async_trait, Request, Response, Status};
use crate::proto::google::protobuf::Empty;
use crate::proto::plugin::grpc_stdio_server::{GrpcStdio, GrpcStdioServer};
use crate::proto::plugin::stdio_data::Channel;
use crate::proto::plugin::{StdioData};
use crate::grpc::error::into_status;

const CONSOLE_POLL_SLEEP_MILLIS: u64 = 500;

pub fn new_server() -> GrpcStdioServer<GrpcStdioImpl> {
    GrpcStdioServer::new(GrpcStdioImpl {})
}

#[derive(Clone)]
pub struct GrpcStdioImpl {}

impl GrpcStdioImpl {
    fn new_combined_stream() -> Result<<Self as GrpcStdio>::StreamStdioStream, Status> {
        log::trace!("new_inner_stream called. Asked for a stream of stdout and stderr");
        log::info!("Gagging stdout and stderr to a buffer for redirection to plugin's host.",);

        let stdoutbuf = BufferRedirect::stdout()
            .context("Failed to create a BufferRedirec from stdout")
            .map_err(|e| e.into())
            .map_err(into_status)?;
        let stdout_stream = GrpcStdioImpl::new_stream("stdout", Channel::Stdout as i32, stdoutbuf);

        let stderrbuf = BufferRedirect::stderr()
            .context("Failed to create a BufferRedirec from stderr")
            .map_err(|e| e.into())
            .map_err(into_status)?;
        let stderr_stream = GrpcStdioImpl::new_stream("stderr", Channel::Stderr as i32, stderrbuf);

        let merged_stream = stdout_stream.merge(stderr_stream);

        Ok(Box::pin(merged_stream))
    }

    fn new_stream(
        stream_name: &'static str,
        channel: i32,
        mut redirected_buf: BufferRedirect,
    ) -> impl Stream<Item = Result<StdioData, Status>> {
        stream! {
            loop {
                log::trace!("beginning next iteration of {} reading and streaming...", stream_name);
                let mut readbuf = String::new();
                match redirected_buf.read_to_string(&mut readbuf) {
                    Ok(len) => match len{
                        0 => {
                            log::trace!("{} had zero bytes. Sleeping to avoid polling...", stream_name);
                            sleep(Duration::from_millis(CONSOLE_POLL_SLEEP_MILLIS)).await;
                        },
                        _ => {
                            log::trace!("Sending {} {} bytes of data: {}", stream_name, len, readbuf);
                            yield Ok(StdioData{
                                channel,
                                data: readbuf.into_bytes(),
                            });
                        },
                    },
                    Err(e) => {
                        log::error!("Error reading {} data: {:?}", stream_name, e);
                        yield Err(Status::unknown(format!("Error reading from Stderr of plugin's process: {:?}", e)));
                    },
                }
            }
        }
    }
}

#[async_trait]
impl GrpcStdio for GrpcStdioImpl {
    type StreamStdioStream =
    Pin<Box<dyn Stream<Item = Result<StdioData, Status>> + Send + 'static>>;

    async fn stream_stdio(
        &self,
        _req: Request<Empty>,
    ) -> Result<Response<Self::StreamStdioStream>, Status> {
        log::trace!("stream_stdio called.");

        let s = GrpcStdioImpl::new_combined_stream()?;

        log::trace!("stream_stdio responding with a stream of StdioData.",);

        Ok(Response::new(s))
    }
}
