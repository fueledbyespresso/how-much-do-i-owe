package authentication

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"how-much-do-i-owe/database"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

var GoogleOauthConfig *oauth2.Config

func ConfigOauth() {
	redirectURL := "https://" + os.Getenv("HOST") + "/oauth/v1/callback"

	if os.Getenv("ENV") == "DEV" {
		redirectURL = "http://localhost:5000/oauth/v1/callback"
	}
	GoogleOauthConfig = &oauth2.Config{
		RedirectURL:  redirectURL,
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}
}

type Error struct {
	StatusCode   int    `json:"status_code"`
	ErrorMessage string `json:"error_msg"`
}

// Routes All the routes created by the package nested in
// oauth/v1/*
func Routes(r *gin.RouterGroup, db *database.DB) {
	r.GET("/login", handleGoogleLogin(db))
	r.GET("/callback", handleGoogleCallback(db))
	r.GET("/logout", handleGoogleLogout(db))
	r.GET("/account", getAccount(db))
	r.GET("/refresh", refreshSession(db))
}

func getSeed() int64 {
	seed := time.Now().UnixNano() // A new random seed (independent from state)
	rand.Seed(seed)
	return seed
}

func handleGoogleLogin(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		state, err := db.SessionStore.Get(c.Request, "state")
		if err != nil {
			c.AbortWithStatusJSON(500, "Server was unable to connect to session database")
			return
		}

		stateString := strconv.FormatInt(getSeed(), 10)
		state.Values["state"] = stateString
		err = state.Save(c.Request, c.Writer)

		if err != nil {
			print("Unable to store state data")
			c.AbortWithStatusJSON(500, "Unable to store state data")
		}

		redirectCallbackURL := GoogleOauthConfig.AuthCodeURL(stateString)
		c.Redirect(http.StatusTemporaryRedirect, redirectCallbackURL)
	}

}

func handleGoogleCallback(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		stateSession, err := db.SessionStore.Get(c.Request, "state")
		if err != nil {
			c.AbortWithStatusJSON(500, "The server was unable to retrieve session state")
			return
		}
		state := fmt.Sprintf("%v", stateSession.Values["state"])
		userData, err := getUserInfo(state, c.Request.FormValue("code"), c.Request)
		if err != nil {
			fmt.Println("Error getting content: " + err.Error())
			c.Redirect(http.StatusTemporaryRedirect, "/")
			return
		}

		stateSession.Options.MaxAge = -1
		_ = stateSession.Save(c.Request, c.Writer)
		fmt.Println(userData)
		if !userExists(userData.Email, db) {
			err = createUser(userData, db)
			if err != nil {
				database.CheckDBErr(err.(*pq.Error), c)
				return
			}
		} else {
			replaceAccessToken(userData, db)
		}

		// set the user information
		session, err := db.SessionStore.Get(c.Request, "session")
		if err != nil {
			c.AbortWithStatusJSON(500, "Server was unable to connect to session database")
		}

		session.Values["GoogleID"] = userData.GoogleID
		session.Values["Email"] = userData.Email
		session.Values["Name"] = userData.Name
		session.Values["Picture"] = userData.Picture

		err = session.Save(c.Request, c.Writer)
		if err != nil {
			fmt.Print("Unable to store session data")
			c.AbortWithStatusJSON(500, "Unable to store session data")
		}

		c.Redirect(http.StatusPermanentRedirect, "/")
	}
}

func createUser(userData User, db *database.DB) error {
	// Prepare the sql query for later
	insert, err := db.Db.Prepare(`INSERT INTO account (email, access_token, google_id, expires_in, picture, name) VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return err
	}

	//Execute the previous sql query using data from the
	// userData struct being passed into the function
	_, err = insert.Exec(userData.Email, userData.AccessToken, userData.GoogleID, userData.ExpiresIn, userData.Picture, userData.Name)

	if err != nil {
		return err
	}
	return nil
}

func userExists(email string, db *database.DB) bool {
	// Prepare the sql query for later
	rows, err := db.Db.Query("SELECT COUNT(*) as count FROM account WHERE email = $1", email)
	PanicOnErr(err)

	return checkCount(rows) > 0
}

func checkCount(rows *sql.Rows) (count int) {
	for rows.Next() {
		err := rows.Scan(&count)
		PanicOnErr(err)
	}
	return count
}

func replaceAccessToken(userData User, db *database.DB) {
	_, err := db.Db.Query("UPDATE account SET access_token=$1,expires_in=$2, picture=$3, name=$4 WHERE email = $5",
		userData.AccessToken, userData.ExpiresIn, userData.Picture, userData.Name, userData.Email)
	if err != nil {
		fmt.Println("Unable to update access token", err)
	}
}

type User struct {
	GoogleID     string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Picture      string    `json:"picture"`
	ExpiresIn    time.Time `json:"expires_in"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
}

func getUserInfo(state string, code string, r *http.Request) (User, error) {
	var userData User

	if state != r.FormValue("state") {
		return userData, fmt.Errorf("invalid oauth state")
	}

	token, err := GoogleOauthConfig.Exchange(context.Background(), code)
	fmt.Println(token.RefreshToken)
	if err != nil {
		return userData, fmt.Errorf("code exchange failed: %s", err.Error())
	}
	//Send access token to Spotify's user api in return for a user's data!
	response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		return userData, fmt.Errorf("failed getting user info: %s", err.Error())
	}

	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return userData, fmt.Errorf("failed reading response body: %s", err.Error())
	}

	err = json.Unmarshal(contents, &userData)
	if err != nil {
		log.Println(err)
	}

	userData.ExpiresIn = token.Expiry
	userData.AccessToken = token.AccessToken
	userData.RefreshToken = token.RefreshToken
	return userData, nil
}

func handleGoogleLogout(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Println("Attempting to expire session")

		session, err := db.SessionStore.Get(c.Request, "session")
		if err != nil {
			c.AbortWithStatusJSON(500, "The server was unable to retrieve this session")
			return
		}

		if session.ID != "" {
			session.Options.MaxAge = -1

			err = session.Save(c.Request, c.Writer)

			if err != nil {
				c.AbortWithStatusJSON(500, "The server was unable to expire this session")
			} else {
				c.JSON(200, `{"successful logout"}`)
			}

		} else {
			c.Redirect(http.StatusTemporaryRedirect, "./")
		}
	}
}

type Account struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	ID      string `json:"user_id"`
}

func refreshSession(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := db.SessionStore.Get(c.Request, "session")
		if err != nil {
			c.AbortWithStatusJSON(500, "The server was unable to retrieve this session")
			return
		}

		if err != nil {
			c.AbortWithStatusJSON(500, "The server was unable to refresh Spotify session")
			return
		}
		if session.ID != "" {
			session.Options.MaxAge = 3600

			err = session.Save(c.Request, c.Writer)
			if err != nil {
				c.AbortWithStatusJSON(500, "The server was unable to refresh this session")
			} else {
				c.JSON(200, "successful refresh")
			}
		} else {
			c.Redirect(http.StatusTemporaryRedirect, "./login")
		}
	}
}

func getAccount(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := db.SessionStore.Get(c.Request, "session")
		if err != nil {
			c.AbortWithStatusJSON(500, "The server was unable to retrieve this session")
			return
		}

		if session.ID != "" {
			// get some session values
			Email := session.Values["Email"]
			EmailStr := fmt.Sprintf("%v", Email)
			Name := session.Values["Name"]
			NameStr := fmt.Sprintf("%v", Name)
			PictureUrl := session.Values["Picture"]
			PictureUrlStr := fmt.Sprintf("%v", PictureUrl)
			GoogleID := session.Values["GoogleID"]
			GoogleIDStr := fmt.Sprintf("%v", GoogleID)
			if err != nil {
				database.CheckDBErr(err.(*pq.Error), c)
				return
			}

			userData := Account{EmailStr, NameStr, PictureUrlStr, GoogleIDStr}

			c.JSON(200, userData)
		} else {
			c.AbortWithStatusJSON(401, "Session not found. Session may be expired or non-existent")
		}
	}
}

func PanicOnErr(err error) {
	if err != nil {
		panic(err)
	}
}
