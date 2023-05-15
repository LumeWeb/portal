use portpicker::Port;

pub struct UniquePort {
    vended_ports: Vec<Port>,
}

impl UniquePort {
    pub fn new() -> Self {
        Self {
            vended_ports: vec![],
        }
    }

    pub fn get_unused_port(&mut self) -> Option<Port> {
        let mut counter = 0;

        loop {
            counter += 1;
            if counter > 1000 {
                // no luck in 1000 tries? Give up!
                return None;
            }

            match portpicker::pick_unused_port() {
                None => return None,
                Some(p) => {
                    if self.vended_ports.contains(&p) {
                        log::trace!("Skipped port: {} because it is in the list of previously vended ports: {:?}", p, self.vended_ports);
                        continue;
                    } else {
                        log::trace!("Vending port: {}", p);
                        self.vended_ports.push(p);
                        return Some(p);
                    }
                }
            }
        }
    }
}

impl Default for UniquePort {
    fn default() -> Self {
        Self::new()
    }
}
