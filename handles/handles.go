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

// Safaricom API Credentials (from environment variables)
var (
	ConsumerKey="fYGoe4lz1jXlAxWVsriCBALe2GXx51AMguzu6pxRZStNFj6x"
	ConsumerSecret="mu6XG0MNTCKAiKvPdZusSRLGgMvpoNFZiw42d7bHbc1haMfpvBWt0GA5VOVGguid"
	ShortCode="174379"
	PassKey="bfb279f9aa9bdbcf158e97dd71a467cd2e0c893059b10f78e6b72ada1ed2c919"
	CallbackURL="https://301f-2c0f-fe38-2251-9037-6665-dee/callback"
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
                log.Printf("Failed to get access token: HTTP %d\n", resp.StatusCode)
                return "", fmt.Errorf("Failed to get access token: HTTP %d", resp.StatusCode)
        }

        var tokenResponse models.AccessTokenResponse
        err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
        if err != nil {
                log.Println("Error decoding access token response:", err)
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
                log.Println("Invalid request body:", err)
                return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
        }

        // Validate Phone number and amount.
        if len(requestData.PhoneNumber) != 12 { // Standard Kenyan phone number format
                log.Println("Invalid phone number:", requestData.PhoneNumber)
                return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid phone number"})
        }

        if requestData.Amount <= 0 {
                log.Println("Invalid amount:", requestData.Amount)
                return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid amount"})
        }

        // Get Access Token
        accessToken, err := GetAccessToken()
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
                log.Println("Error encoding JSON:", err)
                return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode JSON"})
        }

        // Send STK Push Request
        req, err := http.NewRequest("POST", "https://sandbox.safaricom.co.ke/mpesa/stkpush/v1/processrequest", bytes.NewBuffer(jsonData))
        if err != nil {
                log.Println("Error creating STK Push request:", err)
                return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create request"})
        }

        req.Header.Set("Authorization", "Bearer "+accessToken)
        req.Header.Set("Content-Type", "application/json")

        client := &http.Client{}
        resp, err := client.Do(req)
        if err != nil {
                log.Println("Error sending STK Push request:", err)
                return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send STK Push"})
        }
        defer resp.Body.Close()

        var stkResponse models.STKPushResponse
        err = json.NewDecoder(resp.Body).Decode(&stkResponse)

        if err != nil {
                log.Println("Error decoding STK Push response:", err)
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

func CallbackHandler(c *fiber.Ctx) error {
        var callbackData map[string]interface{}
        if err := c.BodyParser(&callbackData); err != nil {
                log.Println("Invalid callback data:", err)
                return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid callback data"})
        }

        log.Println("Callback Response:", callbackData)

        // Update Payment Status
        if body, ok := callbackData["Body"].(map[string]interface{}); ok {
                if stkCallback, ok := body["stkCallback"].(map[string]interface{}); ok {
                        if resultCode, ok := stkCallback["ResultCode"].(float64); ok {
                                if resultCode == float64(0) {
                                        checkoutID, ok := stkCallback["CheckoutRequestID"].(string)
                                        if !ok {
                                                log.Println("CheckoutRequestID not found in callback")
                                                return c.SendStatus(fiber.StatusOK)
                                        }
                                        transactionID, ok := stkCallback["MpesaReceiptNumber"].(string)
                                        if !ok {
                                                log.Println("MpesaReceiptNumber not found in callback")
                                                return c.SendStatus(fiber.StatusOK)
                                        }

                                        var payment models.Payment
                                        db.Db.Where("checkout_id = ?", checkoutID).First(&payment)
                                        payment.TransactionID = transactionID
                                        payment.Status = "Completed"
                                        db.Db.Save(&payment)
                                } else {
                                        log.Println("Callback failed. ResultCode:", resultCode)
                                        if checkoutID, ok := stkCallback["CheckoutRequestID"].(string); ok {
                                                var payment models.Payment
                                                db.Db.Where("checkout_id = ?", checkoutID).First(&payment)
                                                payment.Status = "Failed"
                                                db.Db.Save(&payment)
                                        }

                                }
                        } else {
                                log.Println("ResultCode not found in callback")
                        }
                }
        }

        return c.SendStatus(fiber.StatusOK)
}