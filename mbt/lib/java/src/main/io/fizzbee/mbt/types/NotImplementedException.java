package io.fizzbee.mbt.types;

public class NotImplementedException extends Exception {
    public NotImplementedException() {
        super("Not implemented");
    }

    public NotImplementedException(String message) {
        super(message);
    }

    public NotImplementedException(String message, Throwable cause) {
        super(message, cause);
    }

    public NotImplementedException(Throwable cause) {
        super(cause);
    }
}
