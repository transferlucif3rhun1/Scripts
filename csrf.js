const axios = require("axios");

const TOKEN_ENDPOINT = "https://www.eurowings.com/libs/granite/csrf/token.json";
const HEADER_NAME = "CSRF-Token";

let csrfToken = null;

/**
 * Fetch the CSRF token asynchronously from the server.
 * @returns {Promise<string>} Resolves to the CSRF token.
 */
async function getCsrfToken() {
	try {
		const response = await axios.get(TOKEN_ENDPOINT);
		csrfToken = response.data.token;
		console.log("CSRF token fetched successfully:", csrfToken);
		return csrfToken;
	} catch (error) {
		console.error("Failed to fetch CSRF token:", error.message);
		throw error;
	}
}

/**
 * Adds the CSRF token to a request's headers.
 * @param {object} headers - The headers object to which the CSRF token will be added.
 */
function addCsrfTokenToHeaders(headers = {}) {
	if (csrfToken) {
		headers[HEADER_NAME] = csrfToken;
	} else {
		console.warn("CSRF token is not available. Call getCsrfToken() first.");
	}
	return headers;
}

// Example Usage:
(async () => {
	try {
		// Fetch the CSRF token
		await getCsrfToken();

		// Example: Add CSRF token to a request's headers
		const headers = addCsrfTokenToHeaders({
			"Content-Type": "application/json",
		});

		console.log("Headers with CSRF token:", headers);

		// Example API request with the CSRF token
		const apiResponse = await axios.post(
			"/your/api/endpoint",
			{ key: "value" },
			{ headers }
		);

		console.log("API response:", apiResponse.data);
	} catch (error) {
		console.error("Error during CSRF-protected request:", error.message);
	}
})();
