use std::fmt::{Display, Formatter, Result};

#[derive(Debug, Clone)]
pub enum MbtError {
    NotImplemented(String),
    Other(String),
}

impl MbtError {
    pub fn other<S: Into<String>>(message: S) -> MbtError {
        MbtError::Other(message.into())
    }

    pub fn not_implemented<S: Into<String>>(message: S) -> MbtError {
        MbtError::NotImplemented(message.into())
    }

    pub fn from_err<E: std::error::Error>(error: E) -> MbtError {
        MbtError::Other(format!("{}", error))
    }

    pub fn is_not_implemented(&self) -> bool {
        match self {
            MbtError::NotImplemented(_) => true,
            _ => false
        }
    }
}

impl Display for MbtError {
    fn fmt(&self, f: &mut Formatter) -> Result {
        match self {
            MbtError::NotImplemented(s) => write!(f, "Not Implemented: {}", s),
            MbtError::Other(s) => write!(f, "Execution Error: {}", s),
        }
    }
}

impl std::error::Error for MbtError {}
