package lib

type TriState struct {
    set bool
    value bool
}

func (t TriState) Value(defaultIfNotSet bool) bool {
    if t.set {
        return t.value
    }
    return defaultIfNotSet
}

func (t TriState) IsSetToTrue() bool {
    return t.set && t.value
}

func (t TriState) IsSetToFalse() bool {
    return t.set && !t.value
}

func (t TriState) IsSet() bool {
    return t.set
}

func (t TriState) Set(value bool) {
    t.set = true
    t.value = value
}

func (t TriState) Unset() {
    t.set = false
    t.value = false
}
