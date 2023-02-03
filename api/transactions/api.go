package transactions

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"how-much-do-i-owe/database"
	"math"
	"net/http"
	"strconv"
	"time"
)

type transaction struct {
	ID           string        `json:"id"`
	Amount       float64       `json:"amount"`
	Timestamp    time.Time     `json:"timestamp"`
	Payer        string        `json:"payer" `
	Participants []participant `json:"participants"`
	SplitType    string        `json:"splitType"`
}

type participant struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	DollarShare     float64 `json:"dollarShare"`
	FractionalShare int     `json:"fractionalShare"`
}

// Routes All the routes created by the package nested in
// api/v1/*
func Routes(r *gin.RouterGroup, db *database.DB) {
	r.GET("/transactions", getAllTransactions(db))
	r.GET("/transaction/:id", getTransaction(db))
	r.DELETE("/transaction/:id", deleteTransaction(db))
	r.PATCH("/transaction/:id", modifyTransaction(db))
	r.PUT("/transaction", createTransaction(db))
}

func getAllTransactions(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		googleID, exists := c.Get("GoogleID")
		if !exists {
			c.JSON(http.StatusNotAcceptable, "Active Session Required")
		}
		queryRows, err := db.Db.Query(`SELECT id, payer, timestamp, split_type, a.google_id, email, name FROM transaction
    											LEFT JOIN transaction_participants tp on transaction.id = tp.transaction_id
                                                   JOIN account a on tp.google_id = a.google_id
                                                   WHERE payer=$1 OR a.google_id=$1`, googleID)
		allTrans := make(map[string]transaction)
		for queryRows.Next() {
			var trans transaction
			var parti participant
			err = queryRows.Scan(&trans.ID, &trans.Payer, &trans.Timestamp, &trans.SplitType, &parti.ID, &parti.Email, &parti.Name)
			if err != nil {
				c.AbortWithStatusJSON(500, "The server was unable to get transactions")
			}
			if val, ok := allTrans[trans.ID]; ok {
				val.Participants = append(val.Participants, parti)
				allTrans[trans.ID] = val
			} else {
				allTrans[trans.ID] = trans
			}
		}
		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}

		c.JSON(200, allTrans)
	}
}

func getParticipants(id string, db *database.DB) ([]participant, float64, error) {
	var participants []participant
	var total float64
	query, err := db.Db.Query("SELECT google_id, dollar_share, fractional_share FROM transaction_participants WHERE transaction_id=$2", id)
	for query.Next() {
		var tempPart participant
		err = query.Scan(&tempPart.ID, &tempPart.DollarShare, &tempPart.FractionalShare)
		if err != nil {
			return []participant{}, 0, err
		}
		total += tempPart.DollarShare
		participants = append(participants, tempPart)
	}
	return participants, total, nil
}

func getTransaction(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		googleID, exists := c.Get("GoogleID")
		if !exists {
			c.JSON(http.StatusNotAcceptable, "Active Session Required")
		}
		var trans transaction

		err := db.Db.QueryRow("SELECT id, payer, timestamp, split_type FROM transaction WHERE payer=$1",
			googleID).Scan(&trans.ID, &trans.Payer, &trans.Timestamp, &trans.SplitType)
		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}
		participants, dollarTotal, err := getParticipants(trans.ID, db)
		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}
		trans.Participants = participants
		trans.Amount = dollarTotal

		c.JSON(200, trans)
	}
}

func deleteTransaction(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(400, "Invalid transaction ID")
			return
		}
		googleID := c.GetString("GoogleID")

		if !isPartOfTransaction(db, googleID, id) {
			c.JSON(400, "You are not a participant in this transaction")
		}
		_, err = db.Db.Query("DELETE FROM transaction WHERE id=$1", id)
		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}
		c.JSON(201, id)
	}
}

func isPartOfTransaction(db *database.DB, googleID string, transactionID int) bool {
	count := 0
	err := db.Db.QueryRow(`SELECT count(*) FROM transaction 
    									FULL OUTER JOIN transaction_participants tp 
    									    on transaction.id = tp.transaction_id 
                					WHERE (google_id=$1 OR payer=$1) 
                					  AND transaction_id=$2`, googleID, transactionID).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

func createTransaction(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var trans transaction
		if err := c.ShouldBindJSON(&trans); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(trans.Participants) == 0 {
			c.JSON(http.StatusBadRequest, "MUST HAVE AT LEAST 1 PARTICIPANT!!!")
			return
		}

		switch trans.SplitType {
		case "equal":
			roundedAmount := math.Round(trans.Amount * 100 / 100)
			splitAmount := roundedAmount / float64(len(trans.Participants))
			splitAmount = math.Floor(splitAmount * 100 / 100)

			for i, p := range trans.Participants {
				if i == len(trans.Participants) {
					p.DollarShare = splitAmount
				} else {
					p.DollarShare = splitAmount + math.Mod(roundedAmount, float64(len(trans.Participants)))
				}
			}
		default:
			c.JSON(400, "Invalid split type")
			return
		}
		err := db.Db.QueryRow("INSERT INTO transaction (payer, timestamp, split_type) VALUES ($1, $2) RETURNING id",
			trans.Payer, trans.Timestamp, trans.SplitType).Scan(&trans.ID)
		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}
		// After the transaction is created and ID is generated, add each participant to the DB
		sqlStr := "INSERT INTO transaction_participants (google_id, transaction_id, dollar_share, fractional_share) "
		for _, participant := range trans.Participants {
			sqlStr += fmt.Sprintf("(%s, %s, %.2f, %d),",
				participant.ID, trans.ID, participant.DollarShare, participant.FractionalShare)
		}
		sqlStr = sqlStr[0 : len(sqlStr)-1]
		err = db.Db.QueryRow(sqlStr).Scan()

		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}

		c.JSON(200, nil)
	}
}

func modifyTransaction(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var trans transaction
		if err := c.ShouldBindJSON(&trans); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(trans.Participants) == 0 {
			c.JSON(http.StatusBadRequest, "MUST HAVE AT LEAST 1 PARTICIPANT!!!")
			return
		}
		queryRows, err := db.Db.Query("UPDATE transaction SET payer=$2, timestamp=$3 WHERE id=$4")
		id := 0
		for queryRows.Next() {
			err = queryRows.Scan(&id)
			if err != nil {
				c.AbortWithStatusJSON(500, "The server was unable to update transaction")
			}
		}
		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}
		c.JSON(200, nil)
	}
}
