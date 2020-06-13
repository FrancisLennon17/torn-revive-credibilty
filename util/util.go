package util

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

type AlreadyVotedErr struct{}

func (AlreadyVotedErr) Error() string {
	return "user already voted"
}

//returns the index of the userID in the userList of positive or negative votes
func VotedOnUser(userID string, goodList []string, badList []string) (int, int) {
	positiveVoteExists := -1
	negativeVoteExists := -1
	for index, value := range goodList {
		if value == userID {
			positiveVoteExists = index
		}
	}
	for index, value := range badList {
		if value == userID {
			negativeVoteExists = index
		}
	}

	return positiveVoteExists, negativeVoteExists
}

func VerifyID(idStr string) bool {
	if _, err := strconv.Atoi(idStr); err != nil {
		return false
	}
	return true
}

var validVotes = map[string]bool{
	"positive": true,
	"negative": false,
}

// returns false if vote string isn't valid
func verifyVote(vote string) bool {
	if _, ok := validVotes[vote]; !ok {
		return false
	}
	return true
}

// decodes the user id and vote fields
func DecodeVoteAndUser(encodedStr string) (string, string, error) {
	raw, err := base64.StdEncoding.DecodeString(encodedStr)
	if err != nil {
		return "", "", err
	}
	decodedVote := strings.Split(string(raw), ";")
	if len(decodedVote) != 2 {
		return "", "", errors.New("invalid vote header")
	}
	userID := decodedVote[0]
	if !VerifyID(userID) {
		return "", "", errors.New("user_id encoded field must be a number")
	}
	vote := decodedVote[1]
	if !verifyVote(vote) {
		return "", "", errors.New("invalid vote type")
	}

	return userID, vote, nil
}
