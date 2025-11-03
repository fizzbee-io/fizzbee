package io.fizzbee.mbt.runner;

import io.fizzbee.mbt.types.Model;
import org.junit.jupiter.api.Test;

import java.lang.reflect.Method;
import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * Base test class for model-based testing.
 * Subclasses must implement methods to provide a new model instance and configuration options.
 */
public abstract class FizzBeeTestBase {
    protected static final Map<String, Map<String, Method>> actions = new HashMap<>();

    protected abstract Model newModel();

    protected abstract Map<String, Object> getConfig();

    @Test
    public void testConformanceToModel() throws Exception {
        Model model = newModel();
        int exitCode = Runner.run(model, actions, getConfig());
        assertEquals(0, exitCode);
    }
}
