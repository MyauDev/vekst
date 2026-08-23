import { createConnectTransport } from "@connectrpc/connect-web";

/**
 * The base URL for the browser API.
 *
 * Relative on purpose. In every environment the Ingress routes `/rpc/*` to core
 * and everything else to the web app, so the document and the RPC share an
 * origin and no CORS header is ever required. An absolute URL here would make
 * the deployed environment differ from the local one in exactly the way the
 * Ingress routing exists to prevent.
 *
 * Exported so a test can assert it stays relative.
 */
export const RPC_BASE_URL = "/rpc";

export const transport = createConnectTransport({ baseUrl: RPC_BASE_URL });
