import React, {useEffect, useReducer, useState} from 'react';
import Select from "react-dropdown-select";

export default function CreateTransaction(props: any) {
    const [submittingForm, setSubmittingForm] = useState<boolean>(false)
    const [error, setError] = useState<string | null>(null)
    const [options, setOptions] = useState<any>([{
        id: 1,
        name: "Leanne Graham"
        },
        {
            id:  2,
            name: "Ervin Howell"
        }])
    const [transaction, updateTransaction] = useReducer((prev: any, next: any) => {
        const newTrans = {...prev, ...next}
        const decimalRegex = new RegExp('^\\d+(\.)\\d{0,2}$')
        const numRegex = new RegExp('^\\d+$')
        if(newTrans.amount == ''){
            newTrans.amount = 0
        }
        if(decimalRegex.test(newTrans.amount) || numRegex.test(newTrans.amount)){
            return newTrans
        }

        return prev
    }, {
        amount: 0.01,
        participants: [],
        timestamp: Date.now()
    })
    useEffect(() => {
        console.log(props.contacts)
        let opts = Object.keys(props.contacts).map((key) => {
            if(props.contacts[key].sent && props.contacts[key].received){
                return {id: key, name: props.contacts[key].name}
            }
        })
        setOptions(opts)
    }, [props.contacts])

    function submitForm() {
        setSubmittingForm(true)
        fetch("/api/v1/transaction", {
            method: "PUT",
            body: JSON.stringify(transaction)
        })
            .then((res) => {
                if (res.ok) {
                    setSubmittingForm(false)
                    return res.json()
                }
            })
            .then(
                (result) => {
                    updateTransaction({
                        amount: 0.01,
                        participants: [],
                        timestamp: Date.now()
                    })
                }, (error) => {
                    updateTransaction({
                        amount: 0.01,
                        participants: [],
                        timestamp: Date.now()
                    })
                    setError(error);
                }
            )
    }

    return (
        <form onSubmit={(event) => event.preventDefault()}>
            <label>
                Participants
                <Select multi={true} values={transaction.participants} options={options} labelField="name" valueField="id" onChange={(values) => {
                    updateTransaction({participants: values})
                }} />
            </label>
            <label>
                Amount
                <input value={transaction.amount} onChange={ e=> {
                    updateTransaction({amount: e.target.value})
                }}/>
            </label>
            <button onClick={()=> submitForm()}>Create Expense</button>
        </form>
        )
}