package io.fizzbee.mbt.types;

public interface Model extends RoleMapper {
    void init() throws NotImplementedException;
    void cleanup() throws NotImplementedException;
}
