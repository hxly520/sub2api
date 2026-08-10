<template>
  <svg
    class="home-data-visual-svg"
    viewBox="0 0 760 500"
    preserveAspectRatio="xMidYMid meet"
    aria-hidden="true"
    focusable="false"
  >
    <defs>
      <linearGradient id="home-tech-core" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="var(--visual-core-light)" />
        <stop offset="1" stop-color="var(--visual-core-dark)" />
      </linearGradient>
    </defs>

    <g class="visual-ambient" fill="none" stroke="currentColor" stroke-linecap="round">
      <path d="M32 132C190 38 326 90 466 42S680 88 744 26" />
      <path d="M18 386C170 454 326 402 452 454S670 416 748 350" />
      <path d="M76 246C180 168 272 154 380 168S584 184 690 246" />
    </g>

    <g class="visual-grid" fill="none" stroke="currentColor" stroke-linecap="round">
      <path d="M72 414H688" />
      <path d="M118 382H642" />
      <path d="M165 350H595" />
      <path d="M214 318H546" />
      <path d="M263 286H497" />
      <path d="M380 250L72 414" />
      <path d="M380 250L688 414" />
      <path d="M380 250L184 414" />
      <path d="M380 250L576 414" />
    </g>

    <g class="visual-routes" fill="none" stroke="var(--visual-accent)" stroke-linecap="round">
      <path d="M380 250C320 196 250 132 132 112" />
      <path d="M380 250C380 170 346 104 270 66" />
      <path d="M380 250C438 188 508 104 530 76" />
      <path d="M380 250C464 250 574 188 654 156" />
      <path d="M380 250C458 304 558 354 640 370" />
      <path d="M380 250C302 316 230 368 138 382" />
    </g>

    <g class="visual-orbits" fill="none" stroke="var(--visual-accent)">
      <ellipse cx="380" cy="250" rx="218" ry="86" transform="rotate(-18 380 250)" />
      <ellipse
        cx="380"
        cy="250"
        rx="184"
        ry="126"
        transform="rotate(30 380 250)"
        stroke-dasharray="4 12"
      />
      <ellipse cx="380" cy="250" rx="112" ry="70" transform="rotate(-8 380 250)" />
    </g>

    <g class="visual-core">
      <ellipse cx="380" cy="302" rx="142" ry="35" fill="none" stroke="var(--visual-accent)" />
      <ellipse cx="380" cy="302" rx="98" ry="24" fill="none" stroke="var(--visual-accent)" />
      <path
        class="core-platform"
        d="M380 207L466 252L380 297L294 252Z"
        fill="var(--visual-core-surface)"
        stroke="var(--visual-accent)"
      />
      <path
        class="core-platform core-platform-inner"
        d="M380 222L438 252L380 282L322 252Z"
        fill="none"
        stroke="var(--visual-accent)"
      />

      <g class="core-tower core-tower-left">
        <path class="tower-top" d="M332 190L356 203L332 216L308 203Z" />
        <path class="tower-left" d="M308 203L332 216V257L308 244Z" />
        <path class="tower-right" d="M332 216L356 203V244L332 257Z" />
      </g>

      <g class="core-tower core-tower-right">
        <path class="tower-top" d="M428 174L455 188L428 202L401 188Z" />
        <path class="tower-left" d="M401 188L428 202V258L401 244Z" />
        <path class="tower-right" d="M428 202L455 188V244L428 258Z" />
      </g>

      <g class="core-tower core-tower-center">
        <path class="tower-top" d="M380 132L416 151L380 170L344 151Z" />
        <path class="tower-left" d="M344 151L380 170V252L344 233Z" />
        <path class="tower-right" d="M380 170L416 151V233L380 252Z" />
      </g>
    </g>

    <g class="visual-nodes" fill="var(--visual-accent)">
      <circle cx="132" cy="112" r="4" />
      <circle cx="270" cy="66" r="3" />
      <circle cx="530" cy="76" r="4" />
      <circle cx="654" cy="156" r="3" />
      <circle cx="640" cy="370" r="4" />
      <circle cx="138" cy="382" r="3" />
    </g>

    <g class="visual-pulses" fill="var(--visual-accent)">
      <circle cx="132" cy="112" r="8" />
      <circle cx="530" cy="76" r="9" />
      <circle cx="640" cy="370" r="8" />
    </g>
  </svg>
</template>

<style scoped>
.home-data-visual-svg {
  --visual-accent: #2563eb;
  --visual-core-light: #60a5fa;
  --visual-core-dark: #1d4ed8;
  --visual-core-surface: rgb(255 255 255 / 0.3);
  display: block;
  width: 100%;
  height: auto;
  overflow: visible;
  color: #64748b;
}

:global(.dark) .home-data-visual-svg {
  --visual-accent: #60a5fa;
  --visual-core-light: #93c5fd;
  --visual-core-dark: #2563eb;
  --visual-core-surface: rgb(15 23 42 / 0.48);
  color: #94a3b8;
}

.visual-ambient {
  stroke-width: 1;
  opacity: 0.2;
}

.visual-grid {
  stroke-width: 1;
  opacity: 0.11;
}

.visual-routes {
  stroke-width: 1.2;
  stroke-dasharray: 2 14;
  opacity: 0.34;
  animation: visual-route-flow 8s linear infinite;
}

.visual-orbits {
  stroke-width: 1.35;
  opacity: 0.54;
}

.visual-orbits ellipse:first-child {
  animation: visual-orbit-spin 32s linear infinite;
  transform-box: fill-box;
  transform-origin: center;
}

.visual-orbits ellipse:nth-child(2) {
  animation: visual-orbit-spin-reverse 40s linear infinite;
  transform-box: fill-box;
  transform-origin: center;
}

.visual-orbits ellipse:last-child {
  opacity: 0.8;
}

.visual-core {
  stroke-width: 1.35;
  color: var(--visual-accent);
}

.visual-core ellipse:first-child {
  opacity: 0.34;
  stroke-dasharray: 1 11;
  animation: visual-ring-flow 12s linear infinite;
}

.visual-core ellipse:nth-child(2) {
  opacity: 0.5;
}

.visual-core path {
  stroke-linejoin: round;
}

.core-platform {
  opacity: 0.36;
}

.core-platform-inner {
  opacity: 0.48;
}

.core-tower {
  stroke: var(--visual-accent);
  stroke-width: 1;
  transform-box: fill-box;
  transform-origin: center bottom;
}

.tower-top {
  fill: var(--visual-core-light);
  fill-opacity: 0.58;
}

.tower-left {
  fill: var(--visual-accent);
  fill-opacity: 0.42;
}

.tower-right {
  fill: url(#home-tech-core);
  fill-opacity: 0.76;
}

.core-tower-left {
  opacity: 0.72;
}

.core-tower-right {
  opacity: 0.82;
}

.core-tower-center {
  animation: visual-core-pulse 4.2s ease-in-out infinite;
  transform-box: fill-box;
  transform-origin: center bottom;
}

.visual-nodes circle {
  opacity: 0.85;
}

.visual-pulses circle {
  opacity: 0.14;
  animation: visual-node-pulse 3.6s ease-in-out infinite;
  transform-box: fill-box;
  transform-origin: center;
}

.visual-pulses circle:nth-child(2) {
  animation-delay: 1.1s;
}

.visual-pulses circle:nth-child(3) {
  animation-delay: 2.2s;
}

@keyframes visual-route-flow {
  to {
    stroke-dashoffset: -128px;
  }
}

@keyframes visual-orbit-spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes visual-orbit-spin-reverse {
  to {
    transform: rotate(-360deg);
  }
}

@keyframes visual-ring-flow {
  to {
    stroke-dashoffset: -72px;
  }
}

@keyframes visual-core-pulse {
  0%,
  100% {
    opacity: 0.78;
  }
  50% {
    opacity: 1;
  }
}

@keyframes visual-node-pulse {
  0%,
  100% {
    opacity: 0.08;
    transform: scale(0.72);
  }
  50% {
    opacity: 0.34;
    transform: scale(1.22);
  }
}

@media (prefers-reduced-motion: reduce) {
  .visual-routes,
  .visual-orbits ellipse,
  .visual-core ellipse,
  .core-tower-center,
  .visual-pulses circle {
    animation: none;
  }
}
</style>
