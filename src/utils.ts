import * as os from "os";
import * as path from "path";

export function resolvePath(filePath: string): string {
	if (filePath.startsWith("~")) {
		return path.join(os.homedir(), filePath.slice(1));
	}
	return path.resolve(filePath);
}

export async function retryOperation<T>(
	operation: () => Promise<T>,
	operationName: string,
	maxRetries = 100
): Promise<T> {
	let lastError: Error | undefined;
	for (let i = 0; i < maxRetries; i++) {
		try {
			return await operation();
		} catch (error: any) {
			lastError = error;
			// You can add logic here to check if the error is retryable
			await new Promise((resolve) => setTimeout(resolve, 1000)); // Exponential backoff
		}
	}
	throw new Error(
		`Operation "${operationName}" failed after ${maxRetries} retries. Last error: ${lastError?.message}`
	);
}
