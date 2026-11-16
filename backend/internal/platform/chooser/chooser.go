// Package chooser выбирает участника из переданного состава.
package chooser

import (
	"crypto/rand"
	"errors"
	"math/big"
)

// MemberChooser выбирает участника только из переданного состава.
type MemberChooser interface {
	Choose(members []string) (string, error)
}

var (
	// ErrNoMembers возвращается для пустого состава.
	ErrNoMembers = errors.New("no members to choose from")
	// ErrCandidateNotInList возвращается, когда заданный кандидат вне состава.
	ErrCandidateNotInList = errors.New("candidate is not a member")
)

// Random выбирает участника из криптографически стойкого источника случайности.
type Random struct{}

// Choose возвращает случайного участника переданного состава.
func (Random) Choose(members []string) (string, error) {
	if len(members) == 0 {
		return "", ErrNoMembers
	}
	limit := big.NewInt(int64(len(members)))
	index, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", err
	}
	return members[index.Int64()], nil
}

// Fixed возвращает заранее заданного кандидата.
type Fixed struct {
	Candidate string
}

// Choose возвращает кандидата, если он входит в состав.
func (f Fixed) Choose(members []string) (string, error) {
	if len(members) == 0 {
		return "", ErrNoMembers
	}
	for _, member := range members {
		if member == f.Candidate {
			return f.Candidate, nil
		}
	}
	return "", ErrCandidateNotInList
}
