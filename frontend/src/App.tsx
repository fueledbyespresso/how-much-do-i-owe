import React, {useEffect, useState} from 'react';
import './app.scss';
import CreateTransaction from "./hooks/CreateTransaction";

function App() {
    const [user, setUser] = useState<string | null>(null)
    const [loadingContacts, setLoadingContacts] = useState<boolean>(false)
    const [contacts, setContacts] = useState<any | null>(null)
    const [transactions, setTransactions] = useState<any | null>(null)
    const [loadingTransactions, setLoadingTransactions] = useState<boolean>(false)

    const [error, setError] = useState<string | null>(null)

    useEffect(() => {
        // Refresh session every 10 minutes
        setInterval(refreshSession, 600000)
        // Listen to localstorage changes
        // Update user on change
        window.addEventListener('storage', () => {
            setUser(JSON.parse(localStorage.getItem("user") || ""))
        });

        //Fetch user from api
        fetch("/oauth/v1/account")
            .then((res) => {
                if (res.ok) {
                    return res.json()
                }
            })
            .then(
                (result) => {
                    setUser(result);
                    localStorage.setItem("user", JSON.stringify(result))
                }, (error) => {
                    setUser(null)
                    localStorage.removeItem("user")
                    setError(error);
                }
            )
    }, [])

    function refreshSession() {
        fetch("/oauth/v1/refresh")
            .then((res) => {
                if (!res.ok) {
                    setError(`Unable to refresh session`)
                }
            })
    }

    function getContacts() {
        setLoadingContacts(true)
        fetch("/api/v1/contacts")
            .then((res) => {
                if (res.ok) {
                    setLoadingContacts(false)
                    return res.json()
                }
            })
            .then(
                (result) => {
                    setContacts(result)
                }, (error) => {
                    setContacts(null)
                    setError(error);
                }
            )
    }
    function getTransactions() {
        setLoadingContacts(true)
        fetch("/api/v1/transactions")
            .then((res) => {
                if (res.ok) {
                    setLoadingTransactions(false)
                    return res.json()
                }
            })
            .then(
                (result) => {
                    setTransactions(result)
                }, (error) => {
                    setTransactions(null)
                    setError(error);
                }
            )
    }
    return (
        <div className="App">
            {user ? (
                <div className={"main"}>
                    <div>
                        {contacts != null && Object.keys(contacts).map((key) => {
                            return (
                                <div key={key}>
                                    <div>{contacts[key].name}</div>
                                    <div>{contacts[key].email}</div>
                                    {contacts[key].sent && contacts[key].received && (
                                        <div>FRIENDS!</div>
                                    )}
                                    {contacts[key].sent && !contacts[key].received && (
                                        <div>Waiting for response</div>
                                    )}
                                    {!contacts[key].sent && contacts[key].received && (
                                        <div>Respond to contact request</div>
                                    )}
                                </div>
                            )
                        })}
                        {transactions != null && Object.keys(transactions).map((key) => {
                            return (
                                <div key={key}>
                                    <div>{key}</div>
                                </div>
                            )
                        })}
                        <CreateTransaction contacts={contacts}/>
                        <button onClick={() => getContacts()}>
                            Get Contacts!
                        </button>
                        <button onClick={() => getTransactions()}>
                            Get Transactions!
                        </button>
                    </div>
                </div>
            ) : (
                <div className="not-logged-in">
                    <h1>how much do i owe?</h1>
                    <h2>
                        A digital ledger to keep track of how much your friends owe you.
                    </h2>
                    <label className="spotify-login-button">
                        <a href={"./oauth/v1/login"}>
                            <img src={"/Spotify_Icon_RGB_White.png"}/>
                            Login with Google
                        </a>
                    </label>
                </div>
            )}
        </div>
    );
}

export default App;
