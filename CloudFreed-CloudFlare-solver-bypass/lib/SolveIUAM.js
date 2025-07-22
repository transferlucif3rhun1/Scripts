import delay from "./delay.js";
import { EventEmitter } from 'events';
import CDP from "chrome-remote-interface";

const blockResourceTypes = ["Image", "Font", "Stylesheet", "Other", "Media"];

/**
 * @param {CDP.Client} client
 */
const waitForLoading = (client, requestId, sessionId) => {
  return new Promise((resolve) => {
    client.Network.loadingFinished(({ requestId: finishedRequestId }) => {
      if (finishedRequestId === requestId) {
        resolve();
      }
    }, sessionId);
  });
};

/**
 * @param {CDP.Client} client
 */
async function Solver(client, eventEmitter, sessionId) {
  let keepRunning = true;

  eventEmitter.once('stop', () => {
    keepRunning = false;
  });

  while (keepRunning) {
    try {
      await delay(1000);

      const { nodes } = await client.DOM.getFlattenedDocument(
        {
          depth: -1,
          pierce: true,
        },
        sessionId
      );

      const turnstileNode = nodes.find(
        (node) =>
          node.nodeName === "IFRAME" &&
          node.attributes?.includes("Widget containing a Cloudflare security challenge")
      );

      if (!turnstileNode) {
        continue;
      }

      // Try to get the box model (dimensions) of the node
      const location = await client.DOM.getBoxModel(
        { nodeId: turnstileNode.nodeId },
        sessionId
      );
      const [x1, y1, x2, y2, x3, y3, x4, y4] = location.model.content;

      // Calculate center
      const x = (x1 + x3) / 2;
      const y = (y1 + y3) / 2;

      await delay(1000);

      await client.Input.dispatchMouseEvent(
        {
          type: 'mouseMoved',
          x,
          y,
        },
        sessionId
      );

      // Press the left mouse button
      await client.Input.dispatchMouseEvent(
        {
          type: 'mousePressed',
          x,
          y,
          button: 'left',
          clickCount: 1,
        },
        sessionId
      );

      // Release the left mouse button
      await client.Input.dispatchMouseEvent(
        {
          type: 'mouseReleased',
          x,
          y,
          button: 'left',
          clickCount: 1,
        },
        sessionId
      );
    } catch (error) {
      // Resolve errors silently
    }
  }
}

/**
 * @param {CDP.Client} client
 */
async function SolveIUAM(client, url, html, proxy) {
  return new Promise(async (resolve, reject) => {
    let sessionId;
    const eventEmitter = new EventEmitter();
    const timeout = setTimeout(async () => {
      try {
        await client.DOM.disable(sessionId);
        await client.Page.disable(sessionId);
        await client.Network.disable(sessionId);
        await client.Page.navigate({ url: html }, sessionId);
        resolve({ success: false, code: 408, errormessage: "Request timed out." });
      } catch (error) {
        resolve({ success: false, code: 408, errormessage: "Request timed out." });
      }
    }, 30000); // 30-second timeout

    try {
      const targets = (await client.Target.getTargets()).targetInfos;
      const target = targets.find((target) => target.type === "page");
      const targetId = target.targetId;
      sessionId = (await client.Target.attachToTarget({ targetId, flatten: true })).sessionId;

      await client.DOM.enable(sessionId);
      await client.Page.enable(sessionId);
      await client.Network.enable(undefined, sessionId);

      await client.Network.setRequestInterception({
        patterns: [{ urlPattern: "*" }],
      }, sessionId);

      client.Network.requestIntercepted(async (listener) => {
        try {
          if (listener.authChallenge) {
            if (!proxy.username || !proxy.password) {
              clearTimeout(timeout);
              resolve({
                success: false,
                code: 500,
                errormessage: "Proxy Provided requires a Username & Password, request is missing one or more of these parameters."
              });
              return;
            }
            await client.Network.continueInterceptedRequest({
              interceptionId: listener.interceptionId,
              authChallengeResponse: {
                response: "ProvideCredentials",
                username: proxy.username,
                password: proxy.password,
              },
            }, sessionId);
          } else if (blockResourceTypes.includes(listener.resourceType)) {
            await client.Network.continueInterceptedRequest({
              interceptionId: listener.interceptionId,
              rawResponse: Buffer.from('HTTP/2 404 CloudFreed Stopped media\r\nContent-Type: text/plain\r\nContent-Length: 0\r\n\r\n').toString('base64')
            }, sessionId);
          } else {
            await client.Network.continueInterceptedRequest({
              interceptionId: listener.interceptionId,
            }, sessionId);

            if (listener.request.url === url && listener.request.method === "POST") {
              await waitForLoading(client, listener.requestId, sessionId);

              let cookies = await client.Network.getCookies(undefined, sessionId);
              let cookie = cookies.cookies.find(cookie => cookie.name === 'cf_clearance');

              if (cookie) {
                await client.Runtime.evaluate({ expression: "document.write()" }, sessionId);

                await client.DOM.disable(sessionId);
                await client.Page.disable(sessionId);
                await client.Network.disable(sessionId);
                eventEmitter.emit('stop');

                await client.Page.navigate({ url: html }, sessionId);

                clearTimeout(timeout);
                resolve({ success: true, code: 200, cfClearance: cookie });
              }
            }
          }
        } catch (error) {
          // Resolve errors silently
        }
      });

      await client.Page.navigate({ url }, sessionId);
      await client.Emulation.setFocusEmulationEnabled({ enabled: true }, sessionId);

      Solver(client, eventEmitter, sessionId);
    } catch (error) {
      try {
        if (sessionId) {
          await client.DOM.disable(sessionId);
          await client.Page.disable(sessionId);
          await client.Network.disable(sessionId);
          await client.Page.navigate({ url: html }, sessionId);
        }

        clearTimeout(timeout);
        resolve({
          success: false,
          code: 500,
          error,
          errormessage: "An error occurred on our side. Please check your request or try again later."
        });
      } catch (error) {
        clearTimeout(timeout);
        resolve({
          success: false,
          code: 500,
          error,
          errormessage: "An error occurred on our side. Please check your request or try again later."
        });
      }
    }
  });
}

export default SolveIUAM;
