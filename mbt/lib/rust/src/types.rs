use crate::value::Value;

#[derive(Debug, Clone)]
pub struct Arg {
    pub name: String,
    pub value: Value,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash, Default)]
pub struct RoleId{
    pub role_name: String,
    pub index: i32,
}
