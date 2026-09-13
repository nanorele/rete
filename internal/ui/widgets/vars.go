package widgets

func FindVar[T ~string | ~[]byte](s T, from int) (start, end int, ok bool) {
	if from < 0 {
		from = 0
	}
	for i := from; i+3 < len(s); i++ {
		if s[i] != '{' || s[i+1] != '{' {
			continue
		}
		j := i + 2
		for j < len(s) && s[j] != '{' && s[j] != '}' {
			j++
		}
		if j+1 < len(s) && s[j] == '}' && s[j+1] == '}' {
			return i, j + 2, true
		}
	}
	return 0, 0, false
}
