/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** GraphQL endpoint. Defaults to /query, which Vite proxies to the gateway. */
  readonly VITE_GRAPHQL_URL?: string;
  readonly VITE_GRAPHQL_HTTP?: string;
  readonly VITE_GRAPHQL_WS?: string;
  /** "mock" renders demo data without contacting the gateway. */
  readonly VITE_API_MODE?: "mock" | "live";
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

declare module "*.css" {
  const content: string;
  export default content;
}
