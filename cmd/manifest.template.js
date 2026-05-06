// DANIEL: not sure why that would be needed.
let global;
if (global === undefined) {
  global = globalThis;
}

function injectScript(src, init, callback) {
  if (typeof src !== "string") {
    console.log("🔥🔥🔥 src must be a string", src);
  }

  const js = document.createElement("script");
  js.onerror = (e) =>
    console.log("🔥🔥🔥 script " + src + " failed to load", e);
  js.crossOrigin = "anonymous";
  js.type = "module";

  const currentScript = document.currentScript;
  if (currentScript?.parentNode) {
    currentScript.parentNode.insertBefore(js, currentScript);
  } else {
    document.body.appendChild(js);
  }

  js.src = src;
}

console.log("🔥 [{{ APP }}] LOCAL OVERRIDE");
const baseUrl = "{{ BASE_URL }}";
globalThis.__VITE_BASE_URL__ = baseUrl;

const scripts = [
  `${baseUrl}/runtime.js`,
  `${baseUrl}/polyfills.js`,
  `${baseUrl}/vendor.js`,
  `${baseUrl}/main.js`,
];

for (const src of scripts) {
  injectScript(src);
}

const repeater = function (func, times, interval) {
  let ID = globalThis.setInterval(
    (function (times) {
      return function () {
        if (--times <= 0) globalThis.clearInterval(ID);
        if (func()) {
          globalThis.clearInterval(ID);
        }
      };
    })(times),
    interval
  );
};

// call the repeater with a function as the argument
repeater(
  function () {
    if (globalThis.initializeApp) {
      console.log(`🔥🔥🔥 window.initializeApp exists`);
      globalThis.initializeApp();
      return true;
    } else {
      return false;
    }
  },
  3,
  1000
);
