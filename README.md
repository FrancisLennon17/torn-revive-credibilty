# TORN-REVIVE-CREDIBILITY
Go webservice providing the ability for torn.com player factions to keep a record which users are credible.
Intended to communicate with a browser script that sends request to GET a player's count of positive and negative votes when landing on a players profile. 
The player may then POST via the UI their up/down vote for that user. 

## Running this webservice

- Ensure go is installed and that $GOROOT is exported as the appropriate directory
- In the terminal run `git clone git@github.com:FrancisLennon17/torn-revive-credibilty.git`
- Create a new file in `torn-revive-credibilty/config` called config.go and copy the contents of config.json.example into the file. Update that information with the appropriate configuration
- from the `torn-revive-credibility` directory, run `go run main.go` (can optionally pass `-config={config.json location}`)

## GET /credibility

**Required fields:**

- *(Header)* `target_id` The player you wish to retrieve counts for
- *(Header)* `user_id` Your user ID

**Response:**

- `positives` the count of users that have upvoted this player
- `negatives` the count of users that have downvoted this player

## POST /credibility

**Required fields:**

- *(Header)* `target_id` The player you wish to retrieve counts for
- *(Header)* `vote` [base64 encoded](https://www.base64encode.org/) `{user_id};{positive/negative}` e.g. `1234;positive` (`MTIzNDtwb3NpdGl2ZQ==`) 

Base64 encoded to discourage users from voting with other player's ids, and voting multiple times.  

## DB Script
This webservices reads and writes to the credibility table in a postgres db.
This table has three columns:

1. torn_id
2. positive
3. negative

Edit script below as necessary
```
CREATE TABLE public.credibility
(
    torn_id text COLLATE pg_catalog."default" NOT NULL,
    positive text COLLATE pg_catalog."default",
    negative text COLLATE pg_catalog."default",
    CONSTRAINT credibility_pkey PRIMARY KEY (torn_id)
)

TABLESPACE pg_default;

ALTER TABLE public.credibility
    OWNER to francislennon;
```