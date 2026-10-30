package tui

// cursor tracks a selection and scroll offset over n rows.
type cursor struct {
	pos, off, n int
}

func (c *cursor) setLen(n int) {
	c.n = n
	c.clamp()
}

func (c *cursor) clamp() {
	if c.pos >= c.n {
		c.pos = c.n - 1
	}
	if c.pos < 0 {
		c.pos = 0
	}
	if c.off > c.pos {
		c.off = c.pos
	}
	if c.off < 0 {
		c.off = 0
	}
}

func (c *cursor) move(delta int) {
	c.pos += delta
	c.clamp()
}

func (c *cursor) reset() { c.pos, c.off = 0, 0 }

// window returns the visible [start, end) rows for height h, scrolling the
// offset so the cursor stays visible.
func (c *cursor) window(h int) (int, int) {
	if h <= 0 || c.n == 0 {
		return 0, 0
	}
	if c.pos < c.off {
		c.off = c.pos
	}
	if c.pos >= c.off+h {
		c.off = c.pos - h + 1
	}
	if c.off > c.n-h {
		c.off = max(c.n-h, 0)
	}
	return c.off, min(c.off+h, c.n)
}
