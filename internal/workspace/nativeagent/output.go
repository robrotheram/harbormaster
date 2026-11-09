package nativeagent

// Output bounds native command responses while preserving overflow detection.
type Output struct {
	Data     []byte
	Overflow bool
}

func (b *Output) Write(p []byte) (int, error) {
	n := len(p)
	available := (1 << 20) - len(b.Data)
	if len(p) > available {
		b.Overflow = true
		p = p[:available]
	}
	b.Data = append(b.Data, p...)
	return n, nil
}
