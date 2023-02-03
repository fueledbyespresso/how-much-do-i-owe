package main

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"how-much-do-i-owe/api/contacts"
	"how-much-do-i-owe/api/transactions"
	"how-much-do-i-owe/authentication"
	"how-much-do-i-owe/database"
	"net/http"
	"os"
	"time"
)

func forceSSL() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Header.Get("x-forwarded-proto") != "https" {
			sslUrl := "https://" + c.Request.Host + c.Request.RequestURI
			c.Redirect(http.StatusTemporaryRedirect, sslUrl)
			return
		}
		c.Next()
	}
}

func HasValidSession(dbConnection *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := dbConnection.SessionStore.Get(c.Request, "session")
		if err != nil {
			c.AbortWithStatusJSON(500, "The server was unable to retrieve this session")
			return
		}
		googleID := session.Values["GoogleID"]
		c.Set("GoogleID", googleID)
		c.Next()
	}
}

func createServer(dbConnection *database.DB) *gin.Engine {
	r := gin.Default()
	r.Use(gzip.Gzip(gzip.DefaultCompression))
	if os.Getenv("ENV") != "DEV" {
		r.Use(forceSSL())
	}

	authentication.Routes(r.Group("oauth/v1"), dbConnection)

	v1 := r.Group("api/v1")
	v1.Use(HasValidSession(dbConnection))
	transactions.Routes(v1, dbConnection)
	contacts.Routes(v1, dbConnection)
	r.Use(static.Serve("/", static.LocalFile("./frontend/build", true)))

	return r
}

func main() {
	database.PerformMigrations("file://database/migrations")
	authentication.ConfigOauth()
	db := database.InitDBConnection()
	defer db.Close()

	SStore := database.InitOauthStore()
	// Run a background goroutine to clean up expired sessions from the database.
	defer SStore.StopCleanup(SStore.Cleanup(time.Minute * 5))
	dbConnection := &database.DB{Db: db, SessionStore: SStore}

	r := createServer(dbConnection)

	_ = r.Run()
}
