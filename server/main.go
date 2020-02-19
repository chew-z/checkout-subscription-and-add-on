package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/checkout/session"
	"github.com/stripe/stripe-go/customer"
	"github.com/stripe/stripe-go/webhook"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("godotenv.Load: %v", err)
	}

	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	router := gin.Default()
	router.Use(static.Serve("/", static.LocalFile(os.Getenv("STATIC_DIR"), false)))
	router.POST("/create-checkout-session", handleCreateCheckoutSession)
	router.GET("/checkout-session", handleCheckoutSession)
	router.GET("/public-key", handlePublicKey)
	router.POST("/webhook", handleWebhook)

	addr := ":8080"
	log.Printf("Listening on %s ...", addr)
	router.Run(addr)
}

func handleCreateCheckoutSession(c *gin.Context) {
	var req struct {
		IsBuyingSticker bool `json:"isBuyingSticker"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		log.Printf("json.NewDecoder.Decode: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Items: []*stripe.CheckoutSessionSubscriptionDataItemsParams{
				&stripe.CheckoutSessionSubscriptionDataItemsParams{
					Plan: stripe.String(os.Getenv("SUBSCRIPTION_PLAN_ID")),
				},
			},
		},
		SuccessURL: stripe.String(os.Getenv("DOMAIN") + "/success.html?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(os.Getenv("DOMAIN") + "/cancel.html"),
	}
	if req.IsBuyingSticker {
		params.LineItems = []*stripe.CheckoutSessionLineItemParams{
			&stripe.CheckoutSessionLineItemParams{
				Name:     stripe.String("Pasha e-book"),
				Quantity: stripe.Int64(1),
				Amount:   stripe.Int64(300),
				Currency: stripe.String(string(stripe.CurrencyUSD)),
			},
		}
	}

	session, err := session.New(params)
	if err != nil {
		log.Printf("session.New: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"checkoutSessionId": session.ID})
}

func handlePublicKey(c *gin.Context) {
	publicKey := os.Getenv("STRIPE_PUBLISHABLE_KEY")
	c.JSON(http.StatusOK, gin.H{"publicKey": publicKey})
}

func handleCheckoutSession(c *gin.Context) {

	id := c.PostForm("sessionId")
	if id == "" {
		log.Printf("CheckoutSession ID is missing from URL %s", c.Request.RequestURI)
		c.JSON(http.StatusBadRequest, gin.H{"error": http.StatusText(http.StatusBadRequest)})
		return
	}

	// Fetch the CheckoutSession object from your success page
	// to get details about the order
	session, err := session.Get(id, nil)

	if err != nil {
		log.Printf("An error happened when getting the CheckoutSession %q from Stripe: %v", id, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": http.StatusText(http.StatusBadRequest)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"CheckoutSession": session})
}

func handleWebhook(c *gin.Context) {
	b, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("ioutil.ReadAll: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	event, err := webhook.ConstructEvent(b, c.Request.Header.Get("Stripe-Signature"), os.Getenv("STRIPE_WEBHOOK_SECRET"))
	if err != nil {
		log.Printf("webhook.ConstructEvent: %s", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if event.Type != "checkout.session.completed" {
		return
	}

	cust, err := customer.Get(event.GetObjectValue("customer"), nil)
	if err != nil {
		log.Printf("customer.Get: %v", err)
		return
	}

	if event.GetObjectValue("display_items", "0", "custom") != "" &&
		event.GetObjectValue("display_items", "0", "custom", "name") == "Pasha e-book" {
		log.Printf("🔔 Customer is subscribed and bought an e-book! Send the e-book to %s", cust.Email)
	} else {
		log.Printf("🔔 Customer is subscribed but did not buy an e-book.")
	}
}
