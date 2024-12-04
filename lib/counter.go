package lib

type Counter struct {
    values map[string]int
}

func NewCounter() *Counter {
    return &Counter{values: make(map[string]int)}
}

func (c *Counter) Inc(key string) {
    c.values[key]++
}

func (c *Counter) Get(key string) int {
    return c.values[key]
}

func (c *Counter) List() map[string]int {
    return c.values
}
