package look

// The Hub's working curve is OutCubic (anim.EaseOutCubic) and its pointer
// follow is anim.Chase; the library has both. Only the overshooting curve the
// Hub keeps for its chevrons and outline nodes is the app's.

// EaseOutBackChevron is Qt's Easing.OutBack with overshoot 1.6, the Hub's
// folder chevron turn.
func EaseOutBackChevron(t float32) float32 { return easeOutBack(t, 1.6) }

// EaseOutBackNode is Qt's Easing.OutBack with overshoot 2, the Hub's outline
// node growing as its section is read.
func EaseOutBackNode(t float32) float32 { return easeOutBack(t, 2) }

// easeOutBack is Qt's OutBack: it passes the target by an amount set by s and
// settles back on it, landing exactly at 1.
func easeOutBack(t, s float32) float32 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	t--
	return t*t*((s+1)*t+s) + 1
}
