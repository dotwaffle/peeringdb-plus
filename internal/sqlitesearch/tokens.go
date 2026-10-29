package sqlitesearch

import (
	"strings"
	"unicode"
)

// Tokens covers common standard-analyzer word boundaries. It keeps
// internal apostrophes, dots and connector punctuation, and separates
// Han, hiragana and katakana from adjacent Latin text. Full Lucene
// Unicode word-break and emoji rules remain a registered divergence.
func Tokens(value string) string {
	runes := []rune(strings.ToLower(value))
	var words []string
	var word []rune
	previous := rune(0)
	connector := false
	flush := func() {
		if previous != 0 {
			words = append(words, string(word))
		}
		word = word[:0]
		previous = 0
		connector = false
	}
	for i, r := range runes {
		switch {
		case unicode.IsMark(r) || unicode.Is(unicode.Cf, r):
			if len(word) > 0 {
				word = append(word, r)
			}
		case unicode.Is(unicode.Pc, r):
			if singleton(previous) {
				flush()
			}
			word = append(word, r)
			connector = true
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			if singleton(r) || singleton(previous) || (!connector && scriptBreak(previous, r)) {
				flush()
			}
			word = append(word, r)
			previous = r
			connector = false
		case internalPunctuation(previous, r, nextLetter(runes[i+1:])):
			word = append(word, r)
		default:
			flush()
		}
	}
	flush()
	return strings.Join(words, " ")
}

func singleton(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r)
}

// Katakana forms its own tokens. Connector punctuation can join it to
// other words; scriptBreak only sees adjacent significant letters.
func scriptBreak(a, b rune) bool {
	return a != 0 && katakana(a) != katakana(b)
}

func katakana(r rune) bool {
	return unicode.Is(unicode.Katakana, r) || r == 'ー' || r == 'ｰ'
}

func nextLetter(runes []rune) rune {
	for _, r := range runes {
		if unicode.IsMark(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		return r
	}
	return 0
}

func internalPunctuation(a, punctuation, b rune) bool {
	if singleton(a) || singleton(b) {
		return false
	}
	letters := unicode.IsLetter(a) && unicode.IsLetter(b) &&
		!katakana(a) && !katakana(b)
	numbers := unicode.IsNumber(a) && unicode.IsNumber(b)
	switch punctuation {
	case '\'', '’', '.', '․', '﹒', '．':
		return letters || numbers
	case ':', '·', '·', '״', '‧':
		return letters
	case ',', ';', ';', '٫', '٬', '﹐', '﹔', '，', '；':
		return numbers
	}
	return false
}
