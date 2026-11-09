// Feed retrieval benchmark from the scaling roadmap.
//
//   k6 run -e TOKEN="$TOKEN" tests/load/feed_load.js
//
// The thresholds are the production target for a cached timeline read:
// p95 under 100 ms and p99 under 250 ms with fewer than 1% failed requests.
import { sleep } from 'k6';
import { graphql } from './lib/graphql.js';

export const options = {
  stages: [
    { duration: '1m', target: 500 }, // ramp to 500 virtual users
    { duration: '3m', target: 2000 }, // hold 2,000 concurrent readers
    { duration: '1m', target: 0 }, // ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<100', 'p(99)<250'],
    http_req_failed: ['rate<0.01'],
    // The author field is a dataloader; a regression to N+1 shows up here as
    // a latency distribution with a long tail rather than a hard failure.
    checks: ['rate>0.99'],
  },
};

const FEED_QUERY = `
  query GetFeed($limit: Int, $cursor: String) {
    getFeed(limit: $limit, cursor: $cursor) {
      posts {
        id
        content
        mediaUrl
        likeCount
        hasLiked
        author {
          username
          name
        }
      }
      nextCursor
      hasMore
    }
  }
`;

export default function () {
  graphql(FEED_QUERY, { limit: 20 }, { operation: 'getFeed' });
  sleep(1);
}
