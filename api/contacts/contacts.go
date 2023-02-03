package contacts

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"how-much-do-i-owe/database"
	"net/http"
)

type contact struct {
	Sent     bool   `json:"sent"`
	Received bool   `json:"received"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

// Routes All the routes created by the package nested in
// api/v1/*
func Routes(r *gin.RouterGroup, db *database.DB) {
	r.GET("/contacts", getAllContacts(db))
	r.PUT("/contact/:id", addContact(db))
	r.DELETE("/contact/:id", removeContact(db))
}

func getAllContacts(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		googleID, exists := c.Get("GoogleID")
		if !exists {
			c.JSON(http.StatusNotAcceptable, "Active Session Required")
		}

		queryRows, err := db.Db.Query(`SELECT user_id, contact_id, name, email FROM contact 
				JOIN account a on a.google_id = contact.contact_id WHERE user_id=$1 OR contact_id=$1`, googleID)
		fmt.Println(googleID)
		contacts := make(map[string]contact)
		for queryRows.Next() {
			var senderID string
			var recipientID string
			var name string
			var email string
			err = queryRows.Scan(&senderID, &recipientID, &name, &email)
			if err != nil {
				c.AbortWithStatusJSON(500, "The server was unable to get transactions")
			}
			if senderID == googleID {
				temp := contacts[recipientID]
				temp.Sent = true
				temp.Name = name
				temp.Email = email
				contacts[recipientID] = temp
			} else {
				temp := contacts[senderID]
				temp.Received = true
				temp.Name = name
				temp.Email = email
				contacts[senderID] = temp
			}

		}
		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}

		c.JSON(200, contacts)
	}
}

func addContact(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		googleID, exists := c.Get("GoogleID")
		if !exists {
			c.JSON(http.StatusNotAcceptable, "Active Session Required")
		}
		contactID := c.Param("id")
		receivedContact := 0
		err := db.Db.QueryRow(`INSERT INTO contact (user_id, contact_id) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING
							(SELECT count(*) FROM contact WHERE user_id=$2 AND contact_id=$1)`, googleID, contactID).Scan(&receivedContact)

		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}

		c.JSON(201, receivedContact == 1)
	}
}

func removeContact(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		googleID, exists := c.Get("GoogleID")
		if !exists {
			c.JSON(http.StatusNotAcceptable, "Active Session Required")
		}
		contactID := c.Param("id")
		_, err := db.Db.Query("DELETE FROM contact WHERE (user_id=$1 AND contact_id=$2) OR (user_id=$2 AND contact_id=$1)", googleID, contactID)

		if err != nil {
			database.CheckDBErr(err.(*pq.Error), c)
			return
		}

		c.JSON(201, "success")
	}
}
