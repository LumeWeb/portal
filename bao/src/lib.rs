use abao::encode::Encoder;
use std::io::{Cursor, Write};
#[allow(unused_imports)]
use wasmedge_bindgen::*;
use wasmedge_bindgen_macro::*;

static mut TREE: Option<Vec<u8>> = None;
static mut CURSOR: Option<Cursor<Vec<u8>>> = None;
static mut ENCODER: Option<Encoder<Cursor<Vec<u8>>>> = None;

#[wasmedge_bindgen]
pub unsafe fn init() {
    TREE = Option::Some(Vec::new());
    CURSOR = Option::Some(Cursor::new(TREE.take().unwrap()));
    ENCODER = Option::Some(Encoder::new_outboard(CURSOR.take().unwrap()));
}

#[wasmedge_bindgen]
pub unsafe fn write(v: Vec<u8>) -> Result<u64, String> {
    let encoder = ENCODER.take().unwrap();
    let bytes_written = encoder.to_owned().write(&v).map_err(|e| e.to_string())?;
    ENCODER = Some(encoder); // Restore the value
    Ok(bytes_written as u64)
}

#[wasmedge_bindgen]
pub unsafe fn finalize() -> Vec<u8> {
    let mut encoder = ENCODER.take().unwrap();
    let bytes = encoder.finalize().unwrap().as_bytes().to_vec();
    ENCODER = Some(encoder); // Restore the value
    bytes
}
