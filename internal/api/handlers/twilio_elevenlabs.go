package handlers

import (
	"caller/internal/config"
	"caller/internal/elevenlabs"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var conversations sync.Map

// ConversationData represents a call session with all necessary identifiers and metadata
type ConversationData struct {
	StreamSid      string
	CallSid        string
	ConversationID string
	CallerPhone    string
	UserData       map[string]interface{}
	Direction      string // "inbound" or "outbound"
}

// handleIncomingCall processes webhook requests from twilio when someone calls our number
func HandleIncomingCall() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form data", http.StatusBadRequest)
			return
		}

		// get caller's phone number
		callerPhone := r.FormValue("From")
		log.Printf("[Twilio] Incoming call from: %s", callerPhone)

		userData, err := checkUserExists(callerPhone)
		if err != nil || userData == nil {
			twiml := `<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Say>Sorry, you are not authorized to make this call.</Say>
    <Hangup />
</Response>`
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(twiml))
			return
		}

		// generate twiml
		userDataJSON, _ := json.Marshal(userData)
		twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
	<Connect>
			<Stream url="wss://%s/media-stream">
					<Parameter name="caller_phone" value="%s" />
					<Parameter name="user_data" value="%s" />
					<Parameter name="direction" value="inbound" />
			</Stream>
	</Connect>
</Response>`, r.Host, callerPhone, url.QueryEscape(string(userDataJSON)))

		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(twiml))

	})
}

// handleMediaStream manages bidirectional audio between twilio and elevenlabs
// upgrades http to websocket and routes audio between caller and AI
func HandleMediaStream(upgrader websocket.Upgrader, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[WebSocket] Error upgrading connection: %v", err)
			return
		}
		defer conn.Close()

		log.Println("[Server] Twilio connected to media stream")

		var (
			streamSid       string
			callSid         string
			elevenLabsWs    *websocket.Conn
			callerPhone     = "Unknown"
			userData        map[string]interface{}
			direction       = "inbound" // Default to inbound
			conversation    *ConversationData
			isDisconnecting = false
			disconnectMutex sync.Mutex
		)

		// disconnectCall safely terminates the call, sending required commands to twilio
		// and cleaning up resources to prevent memory leaks
		disconnectCall := func() {
			disconnectMutex.Lock()
			defer disconnectMutex.Unlock()

			if isDisconnecting {
				return
			}

			isDisconnecting = true
			log.Println("[Twilio] Initiating call disconnect")

			if conversation != nil && conversation.ConversationID != "" {
				payload := map[string]interface{}{
					"conversation_id": conversation.ConversationID,
					"phone_number":    conversation.CallerPhone,
					"call_sid":        conversation.StreamSid,
					"direction":       conversation.Direction,
				}

				endpoint := "inbound-calls"
				if conversation.Direction == "outbound" {
					endpoint = "outbound-calls"
				}

				if err := sendConversationWebhook(payload, endpoint, "123123"); err != nil {
					log.Printf("[Webhook] Error sending webhook: %v", err)
				}

				// Remove from conversations map
				conversations.Delete(streamSid)
			}

			// send comamnds to twilio to disconnect
			messages := []map[string]interface{}{
				{"event": "mark_done", "streamSid": streamSid},
				{"event": "clear", "streamSid": streamSid},
				{"event": "twiml", "streamSid": streamSid, "twiml": "<Response><Hangup/></Response>"},
			}

			for _, msg := range messages {
				if err := conn.WriteJSON(msg); err != nil {
					log.Printf("[Twilio] Error sending disconnect message: %v", err)
				}
			}

			// Close WebSocket after a delay
			time.AfterFunc(1*time.Second, func() {
				conn.Close()
			})
		}

		// main websocket message loop
		for {
			// read message from twilio
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[Twilio] WebSocket error: %v", err)
				break
			}

			// skip processing for non-text messages
			if messageType != websocket.TextMessage {
				continue
			}

			var data map[string]interface{}
			if err := json.Unmarshal(message, &data); err != nil {
				log.Printf("[Twilio] Error parsing message: %v", err)
				continue
			}

			event, ok := data["event"].(string)
			if !ok {
				continue
			}

			log.Printf("[Twilio] Received event: %s", event)

			// skip non-stop events if disconnecting
			disconnectMutex.Lock()
			localIsDisconnecting := isDisconnecting
			disconnectMutex.Unlock()

			// ignore events if disconnecting
			if localIsDisconnecting && event != "stop" {
				continue
			}

			switch event {
			case "start":
				// extract stream data from start event
				startData, ok := data["start"].(map[string]interface{})
				if !ok {
					log.Println("[Twilio] Invalid start data")
					continue
				}

				streamSid, _ = startData["streamSid"].(string)
				if callSidVal, ok := startData["callSid"]; ok {
					callSid = callSidVal.(string)
				}

				// extract custom parameters
				customParams, ok := startData["customParameters"].(map[string]interface{})
				if !ok {
					log.Println("[Twilio] No custom parameters")
					continue
				}

				// get caller phone
				if phone, ok := customParams["caller_phone"].(string); ok {
					callerPhone = phone
				}

				// get call direction
				if dir, ok := customParams["direction"].(string); ok {
					direction = dir
				}

				// parse user data
				if userDataStr, ok := customParams["user_data"].(string); ok {
					decodedStr, err := url.QueryUnescape(userDataStr)
					if err != nil {
						log.Printf("[Twilio] Error decoding user data: %v", err)
					} else {
						if err := json.Unmarshal([]byte(decodedStr), &userData); err != nil {
							log.Printf("[Twilio] Error parsing user data: %v", err)
						}
					}
				}

				// create conversation data
				conversation = &ConversationData{
					StreamSid:   streamSid,
					CallSid:     callSid,
					CallerPhone: callerPhone,
					UserData:    userData,
					Direction:   direction,
				}

				conversations.Store(streamSid, conversation)

				log.Printf("[Twilio] Stream started - SID: %s, Phone: %s", streamSid, callerPhone)

				// get a signed URL for ElevenLabs
				signedURL, err := elevenlabs.GetSignedElevenLabsURL(cfg.ElevenLabsAgentID, cfg.ElevenLabsAPIKey)
				if err != nil {
					log.Printf("[ElevenLabs] Error getting signed URL: %v", err)
					disconnectCall()
					continue
				}

				// connect to ElevenLabs
				elevenLabsWs, _, err = websocket.DefaultDialer.Dial(signedURL, nil)
				if err != nil {
					log.Printf("[ElevenLabs] Error connecting: %v", err)
					disconnectCall()
					continue
				}

				// generate configuration based on direction
				isInbound := direction == "inbound"
				config := elevenlabs.GenerateElevenLabsConfig(userData, callerPhone, isInbound)

				// send configuration
				if err := elevenLabsWs.WriteJSON(config); err != nil {
					log.Printf("[ElevenLabs] Error sending config: %v", err)
					disconnectCall()
					continue
				}

				// handle ElevenLabs messages
				go handleElevenLabsMessages(elevenLabsWs, conn, conversation, disconnectCall)

			case "media":
				// forward audio to ElevenLabs
				if elevenLabsWs != nil && !localIsDisconnecting {
					mediaData, ok := data["media"].(map[string]interface{})
					if !ok {
						log.Println("[Twilio] Invalid media data")
						continue
					}

					payload, ok := mediaData["payload"].(string)
					if !ok {
						log.Println("[Twilio] Invalid media payload")
						continue
					}

					// send audio to ElevenLabs
					audioMessage := map[string]string{
						"user_audio_chunk": payload,
					}

					if err := elevenLabsWs.WriteJSON(audioMessage); err != nil {
						log.Printf("[ElevenLabs] Error sending audio: %v", err)
					}
				}

			case "stop":
				// end the conversation and close connections
				if elevenLabsWs != nil {
					elevenLabsWs.WriteJSON(map[string]string{
						"type": "end_conversation",
					})
					elevenLabsWs.Close()
				}

				disconnectCall()
				return
			}
		}
	})
}

// handleOutboundCall initiates calls from our system to specified phone numbers
// accepts number and prompt in json request and returns call status
func HandleOutboundCall(cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Number string `json:"number"`
			Prompt string `json:"prompt"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Number == "" {
			http.Error(w, "Phone number is required", http.StatusBadRequest)
			return
		}

		// create twilio call
		callURL := fmt.Sprintf("https://%s/outbound-call-twiml?prompt=%s&number=%s",
			r.Host, url.QueryEscape(req.Prompt), url.QueryEscape(req.Number))

		params := map[string]string{
			"To":   req.Number,
			"From": cfg.TwilioPhoneNumber,
			"Url":  callURL,
		}

		call, err := createTwilioCall(params, cfg.TwilioAccountSID, cfg.TwilioAuthToken)
		if err != nil {
			log.Printf("[Twilio] Error creating call: %v", err)
			http.Error(w, "Failed to initiate call", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Call initiated",
			"callSid": call["sid"],
		})
	})
}

// handleOutboundCallTwiml generates xml instructions for twilio when setting up outbound calls
// provides parameters needed for the media stream connection
func HandleOutboundCallTwiml() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prompt := r.URL.Query().Get("prompt")
		number := r.URL.Query().Get("number")

		twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream url="wss://%s/media-stream">
            <Parameter name="caller_phone" value="%s" />
            <Parameter name="prompt" value="%s" />
            <Parameter name="direction" value="outbound" />
        </Stream>
    </Connect>
</Response>`, r.Host, number, url.QueryEscape(prompt))

		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(twiml))
	})
}

// private //

// handleElevenLabsMessages processes responses from elevenlabs ai and forwards audio to twilio
func handleElevenLabsMessages(
	elevenLabsWs, twilioWs *websocket.Conn,
	conversation *ConversationData,
	disconnectFunc func(),
) {
	for {
		_, message, err := elevenLabsWs.ReadMessage()
		if err != nil {
			log.Printf("[ElevenLabs] WebSocket error: %v", err)

			// Handle connection closure
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Println("[ElevenLabs] Connection closed normally")
			} else {
				log.Printf("[ElevenLabs] Abnormal connection closure: %v", err)
			}

			disconnectFunc()
			break
		}

		var data map[string]interface{}
		if err := json.Unmarshal(message, &data); err != nil {
			log.Printf("[ElevenLabs] Error parsing message: %v", err)
			continue
		}

		messageType, _ := data["type"].(string)
		log.Printf("[ElevenLabs] Received message type: %s", messageType)

		switch messageType {
		case "conversation_initiation_metadata":
			// Extract conversation ID
			if metadataEvent, ok := data["conversation_initiation_metadata_event"].(map[string]interface{}); ok {
				if conversationID, ok := metadataEvent["conversation_id"].(string); ok {
					conversation.ConversationID = conversationID
					log.Printf("[ElevenLabs] Stored conversation ID: %s", conversationID)

					conversations.Store(conversation.StreamSid, conversation)
				}
			}

		case "audio":
			// Extract audio data and send to Twilio
			var audioBase64 string
			if audioEvent, ok := data["audio_event"].(map[string]interface{}); ok {
				audioBase64, _ = audioEvent["audio_base_64"].(string)
			} else if audio, ok := data["audio"].(map[string]interface{}); ok {
				audioBase64, _ = audio["chunk"].(string)
			}

			if audioBase64 != "" {
				audioData := map[string]interface{}{
					"event":     "media",
					"streamSid": conversation.StreamSid,
					"media": map[string]string{
						"payload": audioBase64,
					},
				}

				if err := twilioWs.WriteJSON(audioData); err != nil {
					log.Printf("[Twilio] Error sending audio: %v", err)
				}
			}

		case "interruption":
			// Clear the current audio
			clearMsg := map[string]interface{}{
				"event":     "clear",
				"streamSid": conversation.StreamSid,
			}

			if err := twilioWs.WriteJSON(clearMsg); err != nil {
				log.Printf("[Twilio] Error sending clear: %v", err)
			}

		case "ping":
			// Respond to ping with pong
			if pingEvent, ok := data["ping_event"].(map[string]interface{}); ok {
				if eventID, ok := pingEvent["event_id"].(string); ok {
					pongResponse := map[string]interface{}{
						"type":     "pong",
						"event_id": eventID,
					}

					if err := elevenLabsWs.WriteJSON(pongResponse); err != nil {
						log.Printf("[ElevenLabs] Error sending pong: %v", err)
					}
				}
			}

		case "end_of_conversation":
			log.Println("[ElevenLabs] End of conversation received")
			disconnectFunc()
			return
		}
	}
}

// createTwilioCall sends api request to twilio to initiate an outbound call
// returns call details including sid for tracking the call status
func createTwilioCall(params map[string]string, accountSid, authToken string) (map[string]interface{}, error) {
	// twilio package can be used here
	client := &http.Client{}

	form := url.Values{}
	for key, value := range params {
		form.Add(key, value)
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls.json", accountSid),
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(accountSid, authToken)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("twilio API error: %s", resp.Status)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// checkUserExists verifies caller authorization based on phone number
// returns user data if authorized or nil if unauthorized
func checkUserExists(phone string) (map[string]interface{}, error) {
	// mocked number authentication for testing purposes
	log.Printf("[Mock] Checking if user exists with phone: %s", phone)

	// return mock user data
	return map[string]interface{}{
		"first_name": "John",
		"last_name":  "Doe",
		"phone":      phone,
	}, nil
}

// sendConversationWebhook notifies external systems about call events
// sends conversation metadata to specified endpoint with authentication
func sendConversationWebhook(payload map[string]interface{}, endpoint string, authToken string) error {
	// mocked webhook call
	log.Printf("[Mock Webhook] Sending payload to %s: %+v", endpoint, payload)
	log.Printf("[Mock Webhook] Using auth token: %s", authToken)

	// simulate successful webhook call
	log.Printf("[Mock Webhook] Successfully sent to %s endpoint", endpoint)
	return nil
}
