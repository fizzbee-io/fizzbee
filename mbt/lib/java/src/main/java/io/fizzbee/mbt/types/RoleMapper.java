package io.fizzbee.mbt.types;

import java.util.Map;

public interface RoleMapper {
    // GetRoles returns the role by its name.
    Map<RoleId, Role> getRoles();
}
