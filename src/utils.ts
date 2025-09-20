import * as os from "os";
import * as path from "path";

//-------------------------------------------------------
// [UI Import]
//-------------------------------------------------------
import { ui } from "./ui/console";

//-------------------------------------------------------
// [Path Utilities]
//-------------------------------------------------------
export function resolvePath(filePath: string): string {
	if (filePath.startsWith("~")) {
		return path.join(os.homedir(), filePath.slice(1));
	}
	return path.resolve(filePath);
}

//-------------------------------------------------------
// [Retry Operation Utility]
//-------------------------------------------------------
export async function retryOperation<T>(operation: () => Promise<T>, operationName: string): Promise<T> {
	let lastError: Error | undefined;
	const networkRetryableErrors = ["EAI_AGAIN", "ECONNRESET", "ETIMEDOUT", "ESOCKETTIMEDOUT", "ENOTFOUND", "EPIPE"];

	const defaultMaxRetries = 100;
	const defaultDelay = 1000; // 1 second

	const networkMaxRetries = Number.MAX_SAFE_INTEGER;
	const networkDelay = 10000; // 10 seconds

	for (let i = 0; i < networkMaxRetries; i++) {
		try {
			return await operation();
		} catch (error: any) {
			lastError = error;
			const errorCode = error?.code || error?.cause?.code;
			const errorMessage = error?.message || "";

			const isNetworkError =
				networkRetryableErrors.includes(errorCode) || errorMessage.includes("getaddrinfo EAI_AGAIN");

			if (isNetworkError) {
				ui.logEvent(
					"WARN",
					`Operation "${operationName}" failed with network error: ${error.message}. Retrying in ${
						networkDelay / 1000
					}s... (Attempt ${i + 1})`
				);
				await new Promise((resolve) => setTimeout(resolve, networkDelay));
			} else {
				// Handle other errors with default retry policy
				if (i < defaultMaxRetries) {
					ui.logEvent(
						"WARN",
						`Operation "${operationName}" failed with non-network error: ${
							error.message
						}. Retrying in ${defaultDelay / 1000}s... (Attempt ${i + 1}/${defaultMaxRetries})`
					);
					await new Promise((resolve) => setTimeout(resolve, defaultDelay));
				} else {
					ui.logEvent(
						"ERROR",
						`Operation "${operationName}" failed after ${defaultMaxRetries} retries for non-network error.`
					);
					throw error; // Re-throw the last error after max retries
				}
			}
		}
	}

	// This part should ideally not be reached if networkMaxRetries is Number.MAX_SAFE_INTEGER
	ui.logEvent("ERROR", `Operation "${operationName}" unexpectedly reached end of network retries.`);
	throw lastError;
}
