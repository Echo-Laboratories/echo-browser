package stealth

// FingerprintExpr is isolated-world JS that dumps the surfaces public
// bot-detector pages look at. It does not write to the page.
const FingerprintExpr = `(() => {
  const webgl = (() => {
    try {
      const c = document.createElement('canvas');
      const gl = c.getContext('webgl') || c.getContext('experimental-webgl');
      if (!gl) return {vendor: null, renderer: null};
      const ext = gl.getExtension('WEBGL_debug_renderer_info');
      return {
        vendor: ext ? gl.getParameter(ext.UNMASKED_VENDOR_WEBGL) : gl.getParameter(gl.VENDOR),
        renderer: ext ? gl.getParameter(ext.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER)
      };
    } catch (e) {
      return {vendor: String(e), renderer: null};
    }
  })();
  const plugins = [];
  try {
    for (let i = 0; i < navigator.plugins.length; i++) {
      plugins.push(navigator.plugins[i].name);
    }
  } catch (e) {}
  return {
    webdriver: navigator.webdriver,
    userAgent: navigator.userAgent,
    appVersion: navigator.appVersion,
    platform: navigator.platform,
    languages: navigator.languages ? [...navigator.languages] : [],
    language: navigator.language,
    pluginCount: navigator.plugins ? navigator.plugins.length : 0,
    plugins: plugins,
    mimeTypes: navigator.mimeTypes ? navigator.mimeTypes.length : 0,
    hardwareConcurrency: navigator.hardwareConcurrency,
    deviceMemory: navigator.deviceMemory ?? null,
    maxTouchPoints: navigator.maxTouchPoints,
    cookieEnabled: navigator.cookieEnabled,
    chromeType: typeof window.chrome,
    chromeRuntime: !!(window.chrome && window.chrome.runtime),
    innerWidth: window.innerWidth,
    innerHeight: window.innerHeight,
    outerWidth: window.outerWidth,
    outerHeight: window.outerHeight,
    screenWidth: screen.width,
    screenHeight: screen.height,
    availWidth: screen.availWidth,
    availHeight: screen.availHeight,
    colorDepth: screen.colorDepth,
    devicePixelRatio: window.devicePixelRatio,
    webglVendor: webgl.vendor,
    webglRenderer: webgl.renderer,
    notificationPermission: (typeof Notification === 'undefined') ? null : Notification.permission,
    cdc: typeof window.cdc_adoQpoasnfa76pfcZLmcfl_,
    playwrightBinding: typeof window.__playwright__binding__,
    pwInitScripts: typeof window.__pwInitScripts,
    puppeteerEval: typeof window.__puppeteer_evaluation_script__,
    seleniumCDC: typeof document.$cdc_asdjflasutopfhvcZLmcfl_,
    dummyFn: typeof window.dummyFn,
    headlessUA: /HeadlessChrome/i.test(navigator.userAgent),
    uaData: navigator.userAgentData ? {
      brands: navigator.userAgentData.brands,
      mobile: navigator.userAgentData.mobile,
      platform: navigator.userAgentData.platform
    } : null
  };
})()`
