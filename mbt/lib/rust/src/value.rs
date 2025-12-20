use std::collections::{HashMap, HashSet};
use std::hash::{Hash, Hasher};

/// Sentinel values for partial state matching in model-based testing.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum Sentinel {
    /// Field should be ignored during comparison
    Ignore,
}

/// Represents a generic value type used in MBT models.
/// Designed to be easy for users — they can directly use standard Rust collections
/// like `HashMap`, `Vec`, and `HashSet`.
#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    Int(i64),
    Str(String),
    Bool(bool),
    Map(HashMap<Value, Value>),
    List(Vec<Value>),
    Set(HashSet<Value>),
    Sentinel(Sentinel),
    None,
}

/// Convenience constant for the IGNORE sentinel
pub const IGNORE: Value = Value::Sentinel(Sentinel::Ignore);

impl Eq for Value {}

impl Hash for Value {
    fn hash<H: Hasher>(&self, state: &mut H) {
        use Value::*;
        // Hash the discriminant so different variants hash differently
        std::mem::discriminant(self).hash(state);
        match self {
            Int(v) => v.hash(state),
            Str(s) => s.hash(state),
            Bool(b) => b.hash(state),
            Sentinel(s) => s.hash(state),
            // Map, List, Set, and None are intentionally not hashed deeply
            // to avoid recursive and unstable hashing. They should not typically
            // be used as HashMap keys.
            Map(_) | List(_) | Set(_) | None => {}
        }
    }
}

impl Value {
    /// Returns the integer value if this is a `Value::Int`, else `None`.
    pub fn as_int(&self) -> Option<i64> {
        if let Value::Int(v) = self {
            Some(*v)
        } else {
            None
        }
    }

    /// Returns the string value if this is a `Value::Str`, else `None`.
    pub fn as_str(&self) -> Option<&str> {
        if let Value::Str(s) = self {
            Some(s)
        } else {
            None
        }
    }

    /// Returns the bool value if this is a `Value::Bool`, else `None`.
    pub fn as_bool(&self) -> Option<bool> {
        if let Value::Bool(b) = self {
            Some(*b)
        } else {
            None
        }
    }

    /// Convenience constructor for maps.
    pub fn from_map(map: HashMap<Value, Value>) -> Self {
        Value::Map(map)
    }

    /// Convenience constructor for lists.
    pub fn from_list(list: Vec<Value>) -> Self {
        Value::List(list)
    }

    /// Convenience constructor for sets.
    pub fn from_set(set: HashSet<Value>) -> Self {
        Value::Set(set)
    }
}

/// Helper to produce a deterministic ordering of map entries
/// (useful before serializing to proto or comparing states).
pub fn sorted_map_entries<'a>(
    map: &'a HashMap<Value, Value>,
) -> Vec<(&'a Value, &'a Value)> {
    let mut entries: Vec<_> = map.iter().collect();
    entries.sort_by(|a, b| {
        format!("{:?}", a.0).cmp(&format!("{:?}", b.0)) // crude but deterministic
    });
    entries
}
