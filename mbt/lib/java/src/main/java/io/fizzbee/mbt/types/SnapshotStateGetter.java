package io.fizzbee.mbt.types;

import java.util.Map;

public interface SnapshotStateGetter {
    Map<String, Object> snapshotState();
}
