package shared

// CappedOutput retains a prefix of command output and records whether any bytes
// were discarded. Write reports the full input length so a full buffer does not
// interrupt the command.
type CappedOutput struct {
	Data     []byte
	Overflow bool
	limit    int
}

func NewCappedOutput(limit int) *CappedOutput {
	return &CappedOutput{limit: limit}
}

func (o *CappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := max(0, o.limit-len(o.Data))
	if n > remaining {
		o.Overflow = true
		p = p[:remaining]
	}
	o.Data = append(o.Data, p...)
	return n, nil
}
