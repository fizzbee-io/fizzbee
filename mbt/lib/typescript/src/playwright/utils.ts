import { Page } from 'playwright';

export interface WaitForDOMSettledOptions {
  /**
   * Timeout in milliseconds to wait after the last DOM mutation before considering the DOM settled.
   * @default 1
   */
  debounceTimeout?: number;
}

/**
 * Waits for the DOM to stop mutating before resolving.
 * Useful after performing actions that trigger UI updates.
 *
 * This utility is designed for use with the AfterActionHook interface in sequential testing mode.
 * It ensures the UI has finished rendering before the next action or state verification.
 *
 * @param page - The Playwright page to observe
 * @param options - Configuration options
 *
 * @example
 * ```typescript
 * import { AfterActionHook } from '@fizzbee/mbt';
 * import { waitForDOMSettled } from '@fizzbee/mbt/playwright';
 *
 * export class MyModelAdapter implements Model, AfterActionHook {
 *   async afterAction(): Promise<void> {
 *     await waitForDOMSettled(this.page);
 *   }
 * }
 * ```
 */
export async function waitForDOMSettled(
  page: Page,
  options: WaitForDOMSettledOptions = {}
): Promise<void> {
  const { debounceTimeout = 1 } = options;

  // The code inside page.evaluate runs in the browser context where DOM APIs are available
  // We use Function constructor to avoid TypeScript checking browser-only APIs
  await page.evaluate((timeout: number) => {
    return new Promise<void>((resolve) => {
      let timer = setTimeout(resolve, timeout);
      // @ts-ignore - MutationObserver and document are available in browser context
      const observer = new MutationObserver(() => {
        clearTimeout(timer);
        timer = setTimeout(resolve, timeout);
      });
      // @ts-ignore - document is available in browser context
      observer.observe(document.body, { attributes: true, childList: true, subtree: true });
    });
  }, debounceTimeout);
}
