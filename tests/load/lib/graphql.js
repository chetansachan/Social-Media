// Shared helpers for the k6 scenarios. Every script posts to the same GraphQL
// endpoint; only the credentials and the document change.
import http from 'k6/http';
import { check } from 'k6';

export const endpoint = __ENV.GRAPHQL_ENDPOINT || 'http://localhost:8080/query';

// TOKEN must be a valid access token for the user the scenario acts as. The
// gateway rejects anonymous mutations and @auth queries.
const token = __ENV.TOKEN || '';

export function graphql(query, variables, tags) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const response = http.post(
    endpoint,
    JSON.stringify({ query, variables }),
    { headers, tags: tags || {} },
  );

  check(response, {
    'http status is 200': (r) => r.status === 200,
    // A 200 response can still carry GraphQL errors, which is how the gateway
    // reports authorization and validation failures.
    'response has no errors': (r) => {
      try {
        return !JSON.parse(r.body).errors;
      } catch (err) {
        return false;
      }
    },
  });

  return response;
}
