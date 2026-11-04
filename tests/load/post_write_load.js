// Post creation write benchmark.
//
//   k6 run -e TOKEN="$TOKEN" -e AUTHOR_ID="$USER_ID" tests/load/post_write_load.js
//
// AUTHOR_ID must be the user the token belongs to; the gateway rejects a
// mismatch with FORBIDDEN before the request reaches post-service.
//
// This scenario only creates posts. Point it at a disposable database or run a
// cleanup job afterwards, because every iteration commits a row and an outbox
// event.
import { check } from 'k6';
import { graphql } from './lib/graphql.js';

const authorId = __ENV.AUTHOR_ID || '';

export const options = {
  scenarios: {
    create_posts: {
      executor: 'constant-arrival-rate',
      // 50 writes/sec for two minutes: roughly the peak write budget of the
      // roadmap for the whole platform, concentrated on one mutation.
      rate: 50,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 100,
      maxVUs: 400,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<300', 'p(99)<600'],
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  },
};

const CREATE_POST = `
  mutation CreatePost($input: CreatePostInput!) {
    CreatePost(input: $input) {
      id
      createdAt
    }
  }
`;

export default function () {
  const input = {
    authorId,
    // Unique content keeps the caption limit and any future dedup logic from
    // turning the benchmark into a validation test.
    content: `load test post vu=${__VU} iter=${__ITER} at=${Date.now()}`,
    mediaUrl: '',
    isPublic: true,
  };

  const response = graphql(CREATE_POST, { input }, { operation: 'CreatePost' });
  check(response, {
    'post id returned': (r) => {
      try {
        return Boolean(JSON.parse(r.body).data.CreatePost.id);
      } catch (err) {
        return false;
      }
    },
  });
}
