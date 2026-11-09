// WebSocket subscription benchmark for the public GraphQL endpoint.
//
//   k6 run -e TOKEN="$TOKEN" -e WS_URL="ws://localhost:8080/query" \
//     tests/load/ws_notifications.js
//
// Requires k6 0.49 or newer for WebSocket subprotocol negotiation. gqlgen
// speaks the graphql-ws protocol: connection_init, connection_ack, start,
// data, complete. The token travels in the connection_init payload so the
// gateway can authorize the subscription before it starts.
import ws from 'k6/ws';
import { check } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const url = __ENV.WS_URL || 'ws://localhost:8080/query';
const token = __ENV.TOKEN || '';
const holdSeconds = Number(__ENV.HOLD_SECONDS || 30);

const frames = new Counter('notification_frames_total');
const acknowledgements = new Trend('notification_connect_ms');

export const options = {
  scenarios: {
    subscribers: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 250 },
        { duration: '2m', target: 1000 },
        { duration: '1m', target: 0 },
      ],
    },
  },
  thresholds: {
    ws_connecting: ['p(95)<1000'],
    notification_frames_total: ['count>0'],
  },
};

const SUBSCRIPTION = `
  subscription OnNotification {
    notificationReceived {
      id
      type
      title
    }
  }
`;

export default function () {
  const response = ws.connect(url, { protocols: ['graphql-ws'] }, function (socket) {
    const connectedAt = Date.now();

    socket.on('open', () => {
      socket.send(JSON.stringify({
        type: 'connection_init',
        payload: { Authorization: `Bearer ${token}` },
      }));
    });

    socket.on('message', (raw) => {
      let frame;
      try {
        frame = JSON.parse(raw);
      } catch (err) {
        return;
      }

      if (frame.type === 'connection_ack') {
        acknowledgements.add(Date.now() - connectedAt);
        socket.send(JSON.stringify({
          id: '1',
          type: 'start',
          payload: { query: SUBSCRIPTION },
        }));
      }

      if (frame.type === 'data') {
        frames.add(1);
      }

      if (frame.type === 'error') {
        // A subscription error should end the virtual user rather than leave
        // it reconnecting in a tight loop.
        socket.close();
      }
    });

    socket.on('error', () => socket.close());
    socket.setTimeout(() => socket.close(), holdSeconds * 1000);
  });

  check(response, { 'websocket upgrade succeeded': (r) => r && r.status === 101 });
}
