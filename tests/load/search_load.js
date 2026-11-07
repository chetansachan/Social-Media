// Search benchmark: post full-text search and user autocomplete.
//
//   k6 run tests/load/search_load.js
//   k6 run -e TERMS="golang,systems,design" tests/load/search_load.js
//
// Both queries run against trigram and tsvector indexes rather than sequential
// scans, so a p99 that degrades while the row count is unchanged points at a
// lost index or a plan regression.
import { graphql } from './lib/graphql.js';

const terms = (__ENV.TERMS || 'golang,system,design,postgres').split(',');

export const options = {
  scenarios: {
    search: {
      executor: 'constant-arrival-rate',
      // Search is read-heavy and cheaper than a feed page, but the trigram
      // planner is pickier, so it gets its own budget rather than sharing feed's.
      rate: 60,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 100,
      maxVUs: 400,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<120', 'p(99)<300'],
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  },
};

const SEARCH_POSTS = `
  query SearchPosts($query: String!, $limit: Int) {
    searchPosts(query: $query, limit: $limit) {
      id
      likeCount
    }
  }
`;

const SEARCH_USERS = `
  query SearchUsers($query: String!, $limit: Int) {
    searchUsers(query: $query, limit: $limit) {
      id
      username
    }
  }
`;

export default function () {
  // Terms rotate so the benchmark hits more than one index shape and does not
  // warm a single hot entry into the cache.
  const term = terms[__ITER % terms.length];

  graphql(SEARCH_POSTS, { query: term, limit: 20 }, { operation: 'searchPosts' });

  // Two-character prefixes are the common autocomplete case and the weakest
  // trigram match, so they are worth measuring separately.
  const prefix = term.slice(0, 2);
  graphql(SEARCH_USERS, { query: prefix, limit: 10 }, { operation: 'searchUsers' });
}
