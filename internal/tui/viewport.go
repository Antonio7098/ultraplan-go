package tui

type viewport struct {
	offset int
	height int
	total  int
}

func newViewport(total, height int) viewport {
	if height < 1 {
		height = 1
	}
	return viewport{height: height, total: total}
}

func (v viewport) MaxOffset() int {
	max := v.total - v.height
	if max < 0 {
		return 0
	}
	return max
}

func (v viewport) Clamp(offset int) int {
	if offset < 0 {
		return 0
	}
	if max := v.MaxOffset(); offset > max {
		return max
	}
	return offset
}

func (v viewport) AtOffset(offset int) viewport {
	v.offset = v.Clamp(offset)
	return v
}

func (v viewport) FollowSelection(start, end int) viewport {
	if start < 0 || end < 0 || v.MaxOffset() == 0 {
		v.offset = 0
		return v
	}
	offset := start
	if end-start+1 < v.height {
		offset = end - v.height + 1
	}
	v.offset = v.Clamp(offset)
	return v
}

func (v viewport) End() int {
	end := v.offset + v.height
	if end > v.total {
		return v.total
	}
	return end
}
