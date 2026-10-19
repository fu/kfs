// The rail down the left edge, the way kfs draws it: grey packets that swell
// and fade (". : | | : .") with gaps between, flowing down one step at a
// time, and now and then a cyan change travelling with them. ASCII only.
(function () {
  const rail = document.getElementById("rail");
  const flow = 300;               // ms per step, as in kfs
  const block = 12, packet = 9;   // a block of 12 steps holds one packet of up to 9
  const glyph = [" ", ".", ":", "|"];

  // splitmix-style hash: the same block always draws the same packet.
  function hash(n) {
    let x = (n * 0x9e3779b1) >>> 0;
    x = Math.imul(x ^ (x >>> 16), 0x85ebca6b) >>> 0;
    x = Math.imul(x ^ (x >>> 13), 0xc2b2ae35) >>> 0;
    return (x ^ (x >>> 16)) >>> 0;
  }

  function height(t) {
    const b = Math.floor(t / block), j = t - b * block;
    const x = hash(b);
    const len = x % (packet + 1);
    let peak = Math.min(3, Math.floor((len + 1) / 2));
    if (((x >>> 8) & 3) === 0) peak = Math.max(1, peak - 1);
    return j >= len ? 0 : Math.min(j + 1, len - j, peak);
  }

  const changes = new Set(); // steps at which a cyan change was born

  function draw() {
    const line = parseFloat(getComputedStyle(rail).lineHeight) || 18;
    const rows = Math.ceil(window.innerHeight / line) + 1;
    const now = Math.floor(Date.now() / flow);
    if (Math.random() < 0.08) changes.add(now);
    let out = "";
    for (let i = 0; i < rows; i++) {
      const t = now - i;
      if (changes.has(t)) {
        const fresh = i < rows / 3;
        out += `<span class="${fresh ? "c" : "c2"}">${fresh ? "|" : ":"}</span>\n`;
      } else {
        out += glyph[height(t)] + "\n";
      }
    }
    for (const t of changes) if (t < now - rows) changes.delete(t);
    rail.innerHTML = out;
  }

  if (!rail) return;
  draw();
  if (!window.matchMedia("(prefers-reduced-motion: reduce)").matches) setInterval(draw, flow);
})();

// Copy buttons next to the install commands.
document.querySelectorAll(".cmd .copy").forEach((btn) => {
  btn.addEventListener("click", async () => {
    const text = btn.parentElement.querySelector("code").textContent.trim();
    try {
      await navigator.clipboard.writeText(text);
      btn.textContent = "ok";
      btn.classList.add("done");
    } catch {
      btn.textContent = "select";
    }
    setTimeout(() => { btn.textContent = "copy"; btn.classList.remove("done"); }, 1500);
  });
});

// Until the recording is there, show the placeholder instead of an empty player.
(function () {
  const video = document.getElementById("demo");
  const placeholder = document.getElementById("demo-placeholder");
  if (!video || !placeholder) return;
  const missing = () => { video.classList.add("missing"); placeholder.classList.add("on"); };
  video.addEventListener("error", missing);
  if (video.networkState === HTMLMediaElement.NETWORK_NO_SOURCE) missing();
})();
