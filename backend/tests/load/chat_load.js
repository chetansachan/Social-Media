// Chat write and read benchmark.
//
//   k6 run -e TOKEN="$TOKEN" -e CONVERSATION_ID="$CONVERSATION_ID" \
//     tests/load/chat_load.js
//
// CONVERSATION_ID must be a thread the token owner belongs to. The scenario
// sends, unread-counts, and reads a page, which is the whole hot path of chat:
// membership check, insert, activity bump, and the inbox query with its three
// batched lookups.
//
// This scenario commits messages. Point it at a disposable database or run a
// cleanup job afterwards.
import { check } from 'k6';
import { graphql } from './lib/graphql.js';

const conversationId = __ENV.CONVERSATION_ID || '';

export const options = {
  scenarios: {
    send_messages: {
      executor: 'constant-arrival-rate',
      // Chat is burstier than posting: a group chat bursts, but the sustained
      // rate stays low. 60/sec is already above a busy product.
      rate: 60,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 100,
      maxVUs: 400,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<150', 'p(99)<400'],
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  },
};

const SEND_MESSAGE = `
  mutation SendMessage($input: SendMessageInput!) {
    SendMessage(input: $input) {
      id
      createdAt
    }
  }
`;

const MY_CONVERSATIONS = `
  query MyConversations($limit: Int) {
    myConversations(limit: $limit) {
      conversations {
        id
        unreadCount
      }
      totalUnread
      hasMore
    }
  }
`;

const GET_MESSAGES = `
  query GetMessages($conversationId: ID!, $limit: Int) {
    getMessages(conversationId: $conversationId, limit: $limit) {
      messages {
        id
        senderId
        createdAt
      }
      nextCursor
      hasMore
    }
  }
`;

export default function () {
  if (!conversationId) {
    throw new Error('CONVERSATION_ID is required: the gateway rejects unknown threads');
  }

  const send = graphql(SEND_MESSAGE, {
    input: {
      conversationId,
      // Unique body keeps a rate limit or dedup rule from turning the
      // benchmark into a validation test.
      content: `load test message vu=${__VU} iter=${__ITER} at=${Date.now()}`,
    },
  }, { operation: 'SendMessage' });

  check(send, {
    'message id returned': (r) => {
      try {
        return Boolean(JSON.parse(r.body).data.SendMessage.id);
      } catch (err) {
        return false;
      }
    },
  });

  // Every second virtual user also reads, so the benchmark exercises the
  // membership-gated read path rather than pure writes.
  if (__VU % 2 === 0) {
    graphql(MY_CONVERSATIONS, { limit: 20 }, { operation: 'MyConversations' });
    graphql(GET_MESSAGES, { conversationId, limit: 50 }, { operation: 'GetMessages' });
  }
}
