package handles

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mpesa/db"
	"mpesa/models"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Safaricom API Credentials
const (
	ConsumerKey    = "YOUR_CONSUMER_KEY"
	ConsumerSecret = "YOUR_CONSUMER_SECRET"
	ShortCode      = "YOUR_SHORTCODE"
	PassKey        = "YOUR_PASSKEY"
	CallbackURL    = "YOUR_CALLBACK_URL"
)

func getAccessToken() (string, error) {
auth:= base64.StdEncoding.EncodeToString([]byte(ConsumerKey + ":"+ConsumerSecret))
req,err:=http.NewRequest("Get","https://sandbox.safaricom.co.ke/oauth/v1/generate?grant_type=client_credentials", nil)
if err != nil {
	return "", err
}


req.Header.Set("Authorization", "Basic "+auth)

client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResponse models.AccessTokenResponse
	err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
	if err != nil {
		return "", err
	}

	return tokenResponse.AccessToken, nil
}



func StkPushHandler(c *fiber.Ctx) error {
	// Parse request body
	var requestData struct {
		PhoneNumber string `json:"phone"`
		Amount      int    `json:"amount"`
	}

	if err := c.BodyParser(&requestData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Get Access Token
	accessToken, err := getAccessToken()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get access token"})
	}

	// Generate Timestamp
	timestamp := time.Now().Format("20060102150405")

		// Encode Password
	password := base64.StdEncoding.EncodeToString([]byte(ShortCode + PassKey + timestamp))

	stkRequest := models.STKPushRequest{
		BusinessShortCode: ShortCode,
		Password:          password,
		Timestamp:         timestamp,
		TransactionType:   "CustomerPayBillOnline",
		Amount:            requestData.Amount,
		PartyA:            requestData.PhoneNumber,
		PartyB:            ShortCode,
		PhoneNumber:       requestData.PhoneNumber,
		CallBackURL:       CallbackURL,
		AccountReference:  "FlutterPayment",
		TransactionDesc:   "Payment from Flutter App",
	}


	jsonData, err := json.Marshal(stkRequest)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode JSON"})
	}
// Send STK Push Request
req, err := http.NewRequest("POST", "https://sandbox.safaricom.co.ke/mpesa/stkpush/v1/processrequest", bytes.NewBuffer(jsonData))
if err != nil {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create request"})
}

req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send STK Push"})
	}
	defer resp.Body.Close()

	var stkResponse models.STKPushResponse
	json.NewDecoder(resp.Body).Decode(&stkResponse)

	payment := models.Payment{
		PhoneNumber: requestData.PhoneNumber,
		Amount:      requestData.Amount,
		CheckoutID:  stkResponse.CheckoutRequestID,
		Status:      "Pending",
	}
	db.Db.Create(&payment)

	return c.JSON(stkResponse)
}


func CallbackHandler(c *fiber.Ctx) error {
	var callbackData map[string]interface{}
	if err := c.BodyParser(&callbackData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid callback data"})
	}


	fmt.Println("Callback Response:", callbackData)

	// Update Payment Status
	if resultCode, exists := callbackData["Body"].(map[string]interface{})["stkCallback"].(map[string]interface{})["ResultCode"]; exists && resultCode == float64(0) {
		checkoutID := callbackData["Body"].(map[string]interface{})["stkCallback"].(map[string]interface{})["CheckoutRequestID"].(string)
		transactionID := callbackData["Body"].(map[string]interface{})["stkCallback"].(map[string]interface{})["MpesaReceiptNumber"].(string)

		var payment models.Payment
		db.Db.Where("checkout_id = ?", checkoutID).First(&payment)
		payment.TransactionID = transactionID
		payment.Status = "Completed"
		db.Db.Save(&payment)
	}

	return c.SendStatus(fiber.StatusOK)
}
