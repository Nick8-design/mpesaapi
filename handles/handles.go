package handles

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"mpesa/db"
	"mpesa/models"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Safaricom API Credentials
var (
	ConsumerKey    = "IipnyUCB5G8vFAH2QVmkgAqiQFhhMgmHHX1jdZukMNlSY33d"
	ConsumerSecret = "zNcEFFKNBVsqR5z5SibNxUfLlX56cmyakcJ31SAWBWPzj4oT5fzvH1jOZkDOS5kt"
	ShortCode      = "174379"
	PassKey        = "bfb279f9aa9bdbcf158e97dd71a467cd2e0c893059b10f78e6b72ada1ed2c919"
	CallbackURL    = "https://mpesaapi.onrender.com/callback"
)

func GetAccessToken() (string, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(ConsumerKey + ":" + ConsumerSecret))

	req, err := http.NewRequest("GET", "https://sandbox.safaricom.co.ke/oauth/v1/generate?grant_type=client_credentials", nil)
	if err != nil {
		log.Println("Error creating access token request:", err)
		return "", err
	}

	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error sending access token request:", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get access token: HTTP %d", resp.StatusCode)
	}

	var tokenResponse models.AccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		log.Println("Error decoding access token response:", err)
		return "", err
	}

	return tokenResponse.AccessToken, nil
}

func StkPushHandler(c *fiber.Ctx) error {
	var requestData struct {
		PhoneNumber string `json:"phone"`
		Amount      int    `json:"amount"`
	}

	if err := c.BodyParser(&requestData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if len(requestData.PhoneNumber) != 12 || requestData.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	accessToken, err := GetAccessToken()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get access token"})
	}

	timestamp := time.Now().Format("20060102150405")
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
		AccountReference:  "House Booking",
		TransactionDesc:   "Booking of house",
	}

	jsonData, err := json.Marshal(stkRequest)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode JSON"})
	}

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
	if err := json.NewDecoder(resp.Body).Decode(&stkResponse); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to decode STK Push response"})
	}

	payment := models.Payment{
		PhoneNumber: requestData.PhoneNumber,
		Amount:      requestData.Amount,
		CheckoutID:  stkResponse.CheckoutRequestID,
		Status:      "Pending",
	}
	db.Db.Create(&payment)

	return c.JSON(stkResponse)
}



// func CallbackHandler(c *fiber.Ctx) error {
// 	// Parse JSON body
// 	var callback models.StkCallback
// 	if err := json.Unmarshal(c.Body(), &callback); err != nil {
// 		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON format"})
// 	}

// 	log.Println("Received Callback:", callback)

// 	// Extract key values
// 	resultCode := callback.Body.StkCallback.ResultCode
// 	resultDesc := callback.Body.StkCallback.ResultDesc
// 	callbackMetadata := callback.Body.StkCallback.CallbackMetadata.Item

// 	// If CallbackMetadata is empty, log and return a failure status
// 	if len(callbackMetadata) == 0 {
// 		fmt.Println("Transaction Failed or Canceled:", resultDesc)
// 		return c.JSON(fiber.Map{
// 			"message": "Transaction Failed or Canceled",
// 			"status":  "failed",
// 		})
// 	}

// 	var mpesaReceiptNumber string
// 	var phoneNumber int64
// 	var amount float64

// 	// Extract values from CallbackMetadata
// 	for _, item := range callbackMetadata {
// 		switch item.Name {
// 		case "MpesaReceiptNumber":
// 			mpesaReceiptNumber = item.Value.(string)
// 		case "PhoneNumber":
// 			phoneNumber = int64(item.Value.(float64))
// 		case "Amount":
// 			amount = item.Value.(float64)
// 		}
// 	}

// 	// Log transaction details
// 	fmt.Printf("Payment received: %+v\n", callback)

// 	// Check if transaction was successful
// 	if resultCode == 0 && mpesaReceiptNumber != "" {
// 		fmt.Println("Transaction Successful:", mpesaReceiptNumber, phoneNumber, amount)
// 		// Save transaction as successful in database
// 	} else {
// 		fmt.Println("Transaction Failed:", resultDesc)
// 	}

// 	// Respond to Safaricom
// 	return c.JSON(fiber.Map{
// 		"message": "Callback received successfully",
// 		"status":  "ok",
// 	})
// }



func CallbackHandler(c *fiber.Ctx) error {
	var callbackData map[string]interface{}

	// Parse the request body
	if err := c.BodyParser(&callbackData); err != nil {
		log.Println("Error parsing callback data:", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid callback data"})
	}

	log.Println("Full Callback Response:", callbackData) // Debugging log

	// Ensure "Body" exists
	body, bodyOk := callbackData["Body"].(map[string]interface{})
	if !bodyOk {
		log.Println("Missing 'Body' field in callback")
		return c.Status(406).JSON(fiber.Map{"error": "Missing 'Body' field"})
	}

	// Ensure "stkCallback" exists
	stkCallback, stkOk := body["stkCallback"].(map[string]interface{})
	if !stkOk {
		log.Println("Missing 'stkCallback' field in callback")
		return c.Status(406).JSON(fiber.Map{"error": "Missing 'stkCallback' field"})
	}

	// Ensure "ResultCode" exists
	resultCode, resultCodeOk := stkCallback["ResultCode"].(float64)
	if !resultCodeOk {
		log.Println("Missing 'ResultCode' field in callback")
		return c.Status(406).JSON(fiber.Map{"error": "Missing 'ResultCode' field"})
	}

	// Extract CheckoutRequestID
	checkoutID, checkoutOk := stkCallback["CheckoutRequestID"].(string)
	if !checkoutOk {
		log.Println("Missing 'CheckoutRequestID' field in callback")
		return c.Status(406).JSON(fiber.Map{"error": "Missing 'CheckoutRequestID' field"})
	}

	// Find the payment record
	var payment models.Payment
	db.Db.Where("checkout_id = ?", checkoutID).First(&payment)

	if resultCode == 0 {
		// Ensure "MpesaReceiptNumber" exists
		transactionID, transactionOk := stkCallback["MpesaReceiptNumber"].(string)
		if !transactionOk {
			log.Println("Missing 'MpesaReceiptNumber' field in callback")
			return c.Status(406).JSON(fiber.Map{"error": "Missing 'MpesaReceiptNumber' field"})
		}

		payment.TransactionID = transactionID
		payment.Status = "Completed"
	} else {
		payment.Status = "Failed"
	}

	// Save updated payment status
	db.Db.Save(&payment)
	return c.SendStatus(fiber.StatusOK)
}
