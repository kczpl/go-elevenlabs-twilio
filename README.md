# Go server for ElevenLabs x Twilio integration

A Go server that connects Twilio's programmable voice API with ElevenLabs AI agents to create an interactive AI phone system. This server handles both inbound and outbound calls, routing audio between the caller and ElevenLabs' conversational AI.

For more information, check out this [article](https://medium.com/@kczpl/building-ai-powered-phone-calls-with-twilio-and-elevenlabs-in-go-f18c169ce699) based on this implementation.


Here's a high-level flow of how these components interact:

```
+----------+                +----------------+                +--------------+
|          |   Audio        |                |    Audio (WS)  |              |
|  Caller  | <----------->  | Twilio Phone   | <----------->  |     Go       |
|          |                | System         |                |   Server     |
+----------+                +----------------+                +--------------+
                                                                    ^
                                                                    |
                                                                    | Audio
                                                                    | (WebSocket)
                                                                    v
                                                              +--------------+
                                                              |              |
                                                              | ElevenLabs   |
                                                              | AI           |
                                                              |              |
                                                              +--------------+
```

## Prerequisites

To follow along, you'll need:
- A Twilio account with a phone number
- An ElevenLabs account with an agent ID and API key

## Features

- Inbound Call Handling: Process incoming calls to your Twilio number
- Outbound Call Capabilities: Initiate calls from your system through Twilio
- Conversational AI: Connect callers with ElevenLabs AI agents
- Customizable Prompts: Configure different prompts for inbound vs outbound calls

## API Endpoints

- POST `/incoming-call`: Webhook endpoint for Twilio to send incoming call notifications
- GET `/media-stream`: WebSocket endpoint for bidirectional audio streaming
- POST `/outbound-call`: Initiate an outbound call
- POST `/outbound-call-twiml`: Generate TwiML for outbound calls

## Environment Variables

```
ELEVENLABS_API_KEY=your_elevenlabs_api_key
ELEVENLABS_AGENT_ID=your_elevenlabs_agent_id
TWILIO_ACCOUNT_SID=your_twilio_account_sid
TWILIO_AUTH_TOKEN=your_twilio_auth_token
TWILIO_PHONE_NUMBER=your_twilio_phone_number
```

