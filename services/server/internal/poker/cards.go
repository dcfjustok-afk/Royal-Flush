package poker

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/cardrank/cardrank"
)

type Card struct {
	Rank string `json:"rank"`
	Suit string `json:"suit"`
}

func NewDeck() []Card {
	ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
	suits := []string{"spades", "hearts", "diamonds", "clubs"}
	deck := make([]Card, 0, 52)
	for _, suit := range suits {
		for _, rank := range ranks {
			deck = append(deck, Card{Rank: rank, Suit: suit})
		}
	}
	return deck
}

func Shuffle(deck []Card) error {
	for i := len(deck) - 1; i > 0; i-- {
		j, err := cryptoIndex(i + 1)
		if err != nil {
			return err
		}
		deck[i], deck[j] = deck[j], deck[i]
	}
	return nil
}

func cryptoIndex(limit int) (int, error) {
	if limit <= 0 {
		return 0, errors.New("shuffle limit must be positive")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func randomID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "-" + encodeHex(raw[:]), nil
}

func randomCode() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	n := binary.LittleEndian.Uint64(raw[:])
	code := make([]byte, 6)
	for i := range code {
		code[i] = alphabet[n%uint64(len(alphabet))]
		n /= uint64(len(alphabet))
	}
	return string(code), nil
}

func encodeHex(raw []byte) string {
	const chars = "0123456789abcdef"
	out := make([]byte, len(raw)*2)
	for i, value := range raw {
		out[i*2] = chars[value>>4]
		out[i*2+1] = chars[value&0x0f]
	}
	return string(out)
}

func Evaluate(pocket, board []Card) (*cardrank.Eval, error) {
	convert := func(cards []Card) ([]cardrank.Card, error) {
		result := make([]cardrank.Card, 0, len(cards))
		for _, card := range cards {
			rank := card.Rank
			if rank == "10" {
				rank = "T"
			}
			suit := map[string]string{"spades": "s", "hearts": "h", "diamonds": "d", "clubs": "c"}[card.Suit]
			if suit == "" {
				return nil, errors.New("invalid card suit")
			}
			value := cardrank.FromString(rank + suit)
			if value == cardrank.InvalidCard {
				return nil, errors.New("invalid card rank")
			}
			result = append(result, value)
		}
		return result, nil
	}
	p, err := convert(pocket)
	if err != nil {
		return nil, err
	}
	b, err := convert(board)
	if err != nil {
		return nil, err
	}
	if len(p) != 2 || len(b) != 5 {
		return nil, errors.New("holdem evaluation requires two pocket cards and five board cards")
	}
	return cardrank.Holdem.Eval(p, b), nil
}
