# 🚀 TCP Custom Protocol Client Guide (ND-JSON Body)

본 문서는 Go 기반 TCP 서버와의 통신을 위한 클라이언트 구현 규약(Protocol)을 정의합니다. 본 서버는 데이터의 구조화를 위해 **ND-JSON** 형식을 채택하고 있습니다.

---

## 1. 연결 및 인증 (Handshake)
서버 접속 직후, 가장 먼저 **인증 JSON**을 전송해야 합니다.

* **포맷:** `JSON + \n` (Newline 필수)
* **구조:**
    ```json
    {
      "name": "your_unique_id",
      "key": "your_auth_key"
    }
    ```

---

## 2. 메시지 전송 규약 (Data Transmission)
모든 메시지는 **[헤더] -> [바디(ND-JSON)] -> [종료 플래그]** 순서로 전송되어야 합니다.

### ① 헤더 (Header)
메시지의 목적지와 식별 정보를 담습니다.
* **규칙:** 반드시 한 줄(`\n`)로 전송
* **필수 필드:** `destination` (수신자 ID), `id` (메시지 식별자)
* **예시:** `{"destination": "userB", "id": "msg_101"}\n`

### ② 바디 (Body - ND-JSON)
본 서버의 바디는 반드시 **ND-JSON(Newline Delimited JSON)** 형식을 따라야 합니다.
* **규칙 1:** 각 JSON 객체는 반드시 한 줄(`\n`)로 구분되어야 합니다.
* **규칙 2:** 바디 내에 여러 개의 JSON 객체를 연속해서 보낼 수 있습니다.
* **예시:** 
    ```json
    {"type":"chat", "text":"hello"}\n
    {"type":"emoji", "code":"heart"}\n
    ```

### ③ 종료 플래그 (End Flag)
서버가 전체 메시지(헤더+바디)의 수신 완료를 감지하는 기준입니다.
* **플래그:** `\0\n`
* **주의:** 바디의 마지막 줄바꿈(`\n`) 직후에 `\0\n`이 붙어야 합니다.

---

## 3. Bun (JavaScript) 구현 예시

```typescript
import { connect } from "bun";

const socket = await connect({
  hostname: "localhost",
  port: 3000,
  socket: {
    open(socket) {
      console.log("Connected. Sending auth...");
      socket.write(JSON.stringify({ name: "userA", key: "pass123" }) + "\n");
    },
    data(socket, buffer) {
      console.log("Received from server:", buffer.toString());
    },
    close(socket) {
      console.log("Connection closed.");
    }
  }
});

/**
 * ND-JSON 형식으로 메시지 전송
 * @param targetId 수신자 ID
 * @param dataObjects 전송할 객체 배열
 */
function sendNDJsonMessage(targetId, dataObjects) {
  // 1. 헤더 전송
  const header = { destination: targetId, id: Date.now().toString() };
  socket.write(JSON.stringify(header) + "\n");

  // 2. 바디 전송 (각 객체를 JSON화 하고 \n 붙여서 전송)
  dataObjects.forEach(obj => {
    socket.write(JSON.stringify(obj) + "\n");
  });

  // 3. 종료 플래그 전송
  socket.write("end\n");
}

// 사용 예시
sendNDJsonMessage("userB", [
  { type: "chat", content: "안녕!" },
  { type: "status", value: "active" }
]);