// Login benchmark. This exercises bcrypt (cost 12) and the refresh-token row,
// so it is expected to be much slower than a cached read; the thresholds guard
// against accidentally lowering or raising the work factor unnoticed.
//
//   k6 run -e LOAD_EMAIL="$EMAIL" -e LOAD_PASSWORD="$PASSWORD" \
//     tests/load/login_load.js
import { check } from 'k6';
import http from 'k6/http';
import { endpoint } from './lib/graphql.js';

const email = __ENV.LOAD_EMAIL || '';
const password = __ENV.LOAD_PASSWORD || '';

export const options = {
  scenarios: {
    login: {
      executor: 'constant-arrival-rate',
      rate: 20,
      timeUnit: '1s',
      duration: '1m',
      preAllocatedVUs: 40,
      maxVUs: 200,
    },
  },
  thresholds: {
    // bcrypt is intentionally expensive; 300 ms leaves headroom for a small
    // pod while still catching a pathological regression.
    http_req_duration: ['p(95)<300', 'p(99)<750'],
    http_req_failed: ['rate<0.01'],
  },
};

const LOGIN = `
  mutation Login($input: LoginInput!) {
    Login(input: $input) {
      accessToken
      refressToken
    }
  }
`;

export default function () {
  const response = http.post(
    endpoint,
    JSON.stringify({ query: LOGIN, variables: { input: { email, password } } }),
    {
      headers: { 'Content-Type': 'application/json' },
      tags: { operation: 'Login' },
    },
  );

  check(response, {
    'http status is 200': (r) => r.status === 200,
    'access token returned': (r) => {
      try {
        return Boolean(JSON.parse(r.body).data.Login.accessToken);
      } catch (err) {
        return false;
      }
    },
  });
}
