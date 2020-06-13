package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"

	"torn-revive-credibility/config"
	"torn-revive-credibility/util"
)

type Env struct {
	DbCon    string
	Hostname string
	Port     int
}

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "./config/config.json", "configuration file path")
	flag.Parse()

	conf, err := config.GetConfig(configFile)
	if err != nil {
		log.Println(err)
	}

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		conf.DBHost, conf.DBPort, conf.DBUser, conf.DBPassword, conf.DBName)

	env := &Env{
		DbCon:    psqlInfo,
		Hostname: conf.Hostname,
		Port:     conf.Port,
	}

	r := mux.NewRouter()
	r.HandleFunc("/credibility", env.fetchCredibility).Methods("GET")
	r.HandleFunc("/credibility", env.rateUser).Methods("POST")
	http.Handle("/", r)

	srv := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", conf.Port),
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	<-c

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("shutting down")
	os.Exit(0)
}

func (e Env) fetchCredibility(w http.ResponseWriter, r *http.Request) {
	targetID := r.Header.Get("target_id")
	userID := r.Header.Get("user_id")
	if targetID == "" || userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "missing required request headers")
		return
	} else if !util.VerifyID(targetID) || !util.VerifyID(userID) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "target_id and user_id fields must be a number")
		return
	}

	log.Printf("\ntarget ID found: %s\n", targetID)
	log.Printf("\nuser ID found: %s\n", userID)

	db, err := sql.Open("postgres", e.DbCon)
	if err != nil {
		log.Fatalf("Error opening DB connection: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer db.Close()

	log.Printf("\nDB connection Opened\n")

	goodList, badList, err := getVoteList(db, targetID)
	if err != nil && err != sql.ErrNoRows {
		log.Fatalf("Error fetching votes: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("\ncurrent votes fetched\n")

	var voted string
	posIndex, negIndex := util.VotedOnUser(userID, goodList, badList)
	if posIndex > -1 {
		voted = "Positive"
	} else if negIndex > -1 {
		voted = "Negative"
	}

	response := struct {
		PositiveVotes int    `json:"positives"`
		NegativeVotes int    `json:"negatives"`
		Voted         string `json:"voted,omitempty"`
	}{
		PositiveVotes: len(goodList),
		NegativeVotes: len(badList),
		Voted:         voted,
	}

	log.Printf("\nresponding with response: %+v\n", response)

	byt, _ := json.Marshal(response)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%+v", string(byt))
}

//fetches the list of ids for the positive and negative voters of the target
func getVoteList(db *sql.DB, tornID string) ([]string, []string, error) {
	var good []string
	var bad []string
	query := sq.
		Select("positive, negative").
		From("credibility").
		Where(sq.Eq{`torn_id`: tornID}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return good, bad, err
	}

	log.Printf("\nfetching from table with query: %+v with args: %+v\n", sqlStr, args)
	var negativeVotes string
	var positiveVotes string
	row := db.QueryRow(sqlStr, args...)
	parseErr := row.Scan(&positiveVotes, &negativeVotes)
	if parseErr != nil {
		return good, bad, parseErr
	}

	if len(positiveVotes) > 0 {
		good = strings.Split(positiveVotes, ";")
	}
	if len(negativeVotes) > 0 {
		bad = strings.Split(negativeVotes, ";")
	}

	return good, bad, nil
}

func (e Env) rateUser(w http.ResponseWriter, r *http.Request) {
	targetID := r.Header.Get("target_id")
	voteEncoded := r.Header.Get("vote")
	if targetID == "" || voteEncoded == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "missing required request headers")
		return
	} else if !util.VerifyID(targetID) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "target_id field must be a number")
		return
	}

	log.Printf("\nvalid target_id found\n")

	userID, vote, err := util.DecodeVoteAndUser(voteEncoded)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "error parsing encoded vote field: %v", err)
		return
	}
	if userID == targetID {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "cant vote for yourself")
	}

	log.Printf("\nvalid vote parsed\n")
	log.Printf("\nopening db connection\n")

	db, err := sql.Open("postgres", e.DbCon)
	if err != nil {
		log.Fatalf("Error opening DB connection: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer db.Close()

	log.Printf("\nupdating credibility table\n")

	rateErr := rate(db, targetID, userID, vote)
	if rateErr != nil && rateErr.Error() != "user already voted" {
		log.Fatalf("Error rating target: %+v", rateErr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if rateErr != nil && rateErr.Error() == "user already voted" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Nothing to update, user already voted")
	}

	w.WriteHeader(http.StatusOK)
}

// rates the target with the choice of the user
func rate(db *sql.DB, targetID, userID, vote string) error {
	//fetch current votes for that user
	goodList, badList, err := getVoteList(db, targetID)
	if err != nil && err != sql.ErrNoRows {
		return err
	} else if err != nil && err == sql.ErrNoRows {
		//create row if it doesn't exist
		createErr := createCredibilityRowForTarget(db, userID, targetID, vote)
		if createErr != nil {
			return createErr
		}
	}

	// if voted already, swap vote or return "alreadyVotedErr"
	votedPositive, votedNegative := util.VotedOnUser(targetID, goodList, badList)
	if vote == "positive" {
		if votedPositive != -1 {
			return util.AlreadyVotedErr{}
		} else if votedNegative != -1 {
			badList = append(badList[:votedNegative], badList[votedNegative+1:]...)
			removeVoteErr := updateCredibility(db, badList, targetID, "negative")
			if removeVoteErr != nil {
				return removeVoteErr
			}
		}
		goodList = append(goodList, userID)
		return updateCredibility(db, goodList, targetID, vote)
	} else if vote == "negative" {
		if votedNegative != -1 {
			return util.AlreadyVotedErr{}
		} else if votedPositive != -1 {
			goodList = append(goodList[:votedPositive], goodList[votedPositive+1:]...)
			removeVoteErr := updateCredibility(db, goodList, targetID, "positive")
			if removeVoteErr != nil {
				return removeVoteErr
			}
		}
		badList = append(badList, userID)
		return updateCredibility(db, badList, targetID, vote)
	}

	return errors.New("Invalid choice")
}

// updates the credibility table with the new list of positive/negative voters
func updateCredibility(db *sql.DB, voters []string, targetID, vote string) error {
	votersStr := strings.Join(voters, ";")
	updateSql, args, updateErr := sq.Update("credibility").
		Set(vote, votersStr).
		Where(sq.Eq{"torn_id": targetID}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if updateErr != nil {
		return updateErr
	}
	log.Printf("\nupdating credibility table sql query:%v Args: %v\n", updateSql, args)
	_, err := db.Exec(updateSql, args...)
	return err
}

// inserts into the credibility table with the user vote
func createCredibilityRowForTarget(db *sql.DB, userID, targetID, vote string) error {
	positive := ""
	negative := ""
	if vote == "positive" {
		positive = userID
	} else if vote == "negative" {
		negative = userID
	}
	insertSql, args, insertErr := sq.Insert("credibility").
		Columns("torn_id", "positive", "negative").
		Values(targetID, positive, negative).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if insertErr != nil {
		return insertErr
	}
	log.Printf("\n creating row in credibility table with sql query:%v Args: %v\n", insertSql, args)
	_, err := db.Exec(insertSql, args...)
	return err
}
