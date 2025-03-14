// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package rpc defines the client-server interaction for remote procedure calls (RPC) within the system.

// Related packages

// - jsoncodec: Defines methods for reading and writing RPC messages in JSON format, enabling serialization and deserialization.
// - params: Defines the structs for json RPC requests

// Example sequence flow for a rpc interaction

// 1. Registering the API Facade:
//    During the initialization phase of the API server, the API facade is registered with the root object's registry
//    using a method such as: registry.MustRegister("ModelManager", 10, ...)
// 2. Command Execution:
//    When a user executes a command (e.g., AddModelCommand), an API request containing
//    a struct from the `params` package is sent to the API client with the appropriate API facade.
// 3. Sending the RPC Request:
//    The RPC client sends the following as part of its request to the RPC server:
//    - Facade name
//    - Version number
//    - Parameter struct (from the `params` package)
// 4. Message Encoding (jsoncodec package):
//    Before sending the request, the message is encoded using a method from the `jsoncodec` package.
// 5. Asynchronous Request Handling:
//    The RPC server runs asynchronously, reading requests from the RPC client as they arrive.
// 6. Message Decoding (jsoncodec package):
//    Upon receiving a request, the RPC server decodes the message using a method from the `jsoncodec` package.
// 7. Resolving and Executing the Request:
//    The RPC server finds the appropriate RPC method using the facade name and version number.
//    It then executes the corresponding API function using `conn.runRequest`.

// Key components

// - Conn: Represents an RPC connection, managing client and server state.
// - Call: Represents an active RPC call, containing a Request, Response, and error details.
// - Request: Represents the structure of the RPC request, including:
//   - Id: (String) The ID of a watched object.
//   - Type: (String) The type of object to act on.
//   - Version: (Number) The version of the Type we will be acting on.
//   - Action: (String) The action to perform on the object.
// - Response (outMsgV1): Represents the structure of the RPC response, including:
//   - RequestId: (Number) The sequence number of the request.
//   - Type: (String) The type of the response.
//   - Version: (Number) The version of the response.
//   - Id: (String) The unique identifier for the response.
//   - Request: (String) The original request as a string.
//   - Params: (Interface) Optional parameters associated with the request.
//   - Error: (String) An error message, if any.
//   - ErrorCode: (String) The code of the error, if any.
//   - ErrorInfo: (Map) Additional information about the error, if any.
//   - Response: (Interface) An optional response, which can be a JSON structure.

// Client-server interaction implementation details

// Client-side RPC call:
// 1. Create an RPC Call object to encapsulate the request details.
// 2. Use conn.Call to execute the RPC call, sending the request to the server.
// 3. The server processes the request and sends a response, which is stored in the Response field.

// Server-side RPC request handling:
// 1. Configure the server by calling conn.Serve to:
//    - Register the root object for handling requests.
//    - Optionally configure error transformation and request recording.
// 2. Start the connection using conn.Start, which begins the server loop to handle requests.
// 3. The server loop (conn.loop) reads incoming requests, processes them, and sends responses:
//    - Reads the request header and determines if it’s a request or response.
//    - For requests, resolves the appropriate method and executes it via conn.runRequest.
//    - Serializes the result or error and sends it back to the client.

package rpc
