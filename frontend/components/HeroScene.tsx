"use client";

import { useEffect, useRef } from "react";
import * as THREE from "three";

// The sign-in hero: the market as a breathing globe of points. Every couple of seconds a bid is
// "spotted": a shockwave runs across the surface from that point and the name floats up next to
// it. The names are illustrative; nothing here talks to the backend.
const NAMES = ["neon", "vault", "orbit", "nova", "cipher", "lumen", "atlas", "onyx", "drift", "pixel", "echo", "flux", "zen", "aura", "relay", "quant"];
const PINGS = 4; // shockwaves alive at once
const PING_EVERY = 1.7; // seconds
const PING_LIFE = 3.2;

const vertex = /* glsl */ `
  uniform float uTime;
  uniform float uSize;
  uniform float uCamZ; // camera distance: depth and point size are relative to it
  uniform vec3 uPing[${PINGS}];
  uniform float uPingT[${PINGS}];
  attribute float aSeed;
  varying float vGlow;
  varying float vDepth;

  // cheap smooth noise: enough to make the surface breathe without a texture
  float wobble(vec3 p, float t) {
    return sin(p.x * 3.1 + t * 0.7) * sin(p.y * 2.7 - t * 0.5) * sin(p.z * 3.3 + t * 0.6);
  }

  void main() {
    vec3 dir = normalize(position);
    float r = 1.0 + 0.045 * wobble(dir * 1.6, uTime) + 0.012 * sin(aSeed * 40.0 + uTime * 2.0);
    float glow = 0.0;
    for (int i = 0; i < ${PINGS}; i++) {
      float age = uTime - uPingT[i];
      if (age < 0.0 || age > ${PING_LIFE.toFixed(1)}) continue;
      float d = acos(clamp(dot(dir, uPing[i]), -1.0, 1.0));     // distance along the surface
      float front = age * 0.95;                                  // the ring travels outwards
      float ring = exp(-pow((d - front) * 9.0, 2.0)) * (1.0 - age / ${PING_LIFE.toFixed(1)});
      float core = exp(-d * 14.0) * exp(-age * 2.2);
      glow += ring * 0.9 + core * 0.9;
      r += ring * 0.07 + core * 0.06;
    }
    vGlow = glow;
    vec4 mv = modelViewMatrix * vec4(dir * r, 1.0);
    vDepth = clamp((-mv.z - (uCamZ - 1.0)) / 2.0, 0.0, 1.0);     // 0 = front, 1 = back of the globe
    gl_PointSize = uSize * (uCamZ / 3.6) * (1.0 + glow * 1.4) * (0.6 + 0.4 * fract(aSeed * 13.7)) / -mv.z;
    gl_Position = projectionMatrix * mv;
  }
`;

const fragment = /* glsl */ `
  uniform vec3 uBase;
  uniform vec3 uHot;
  varying float vGlow;
  varying float vDepth;
  void main() {
    float d = length(gl_PointCoord - 0.5);
    if (d > 0.5) discard;
    float soft = smoothstep(0.5, 0.0, d);
    vec3 col = mix(uBase, uHot, clamp(vGlow, 0.0, 1.0));
    float a = soft * mix(1.0, 0.22, vDepth) + vGlow * 0.7 * soft;
    gl_FragColor = vec4(col, a);
  }
`;

export function HeroScene({ className, density = 9000 }: { className?: string; density?: number }) {
  const host = useRef<HTMLDivElement>(null);
  const labels = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = host.current;
    const tagLayer = labels.current;
    if (!el || !tagLayer) return;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const dark = document.documentElement.classList.contains("dark");

    let renderer: THREE.WebGLRenderer;
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: "low-power" });
    } catch {
      return; // no WebGL: the panel simply stays calm
    }
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.setClearColor(0x000000, 0);
    el.prepend(renderer.domElement); // under the name tags
    renderer.domElement.style.cssText = "position:absolute;inset:0;width:100%;height:100%";

    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(38, 1, 0.1, 50);
    camera.position.set(0, 0, 3.6);
    const world = new THREE.Group();
    world.rotation.x = 0.35;
    scene.add(world);

    // the globe: points on a Fibonacci sphere, evenly spread
    const n = density;
    const pos = new Float32Array(n * 3);
    const seed = new Float32Array(n);
    const golden = Math.PI * (3 - Math.sqrt(5));
    for (let i = 0; i < n; i++) {
      const y = 1 - (i / (n - 1)) * 2;
      const rad = Math.sqrt(1 - y * y);
      const th = golden * i;
      pos.set([Math.cos(th) * rad, y, Math.sin(th) * rad], i * 3);
      seed[i] = Math.random();
    }
    const geo = new THREE.BufferGeometry();
    geo.setAttribute("position", new THREE.BufferAttribute(pos, 3));
    geo.setAttribute("aSeed", new THREE.BufferAttribute(seed, 1));
    const pings = Array.from({ length: PINGS }, () => new THREE.Vector3(0, 1, 0));
    const pingT = new Array(PINGS).fill(-100);
    const mat = new THREE.ShaderMaterial({
      vertexShader: vertex,
      fragmentShader: fragment,
      transparent: true,
      depthWrite: false,
      blending: dark ? THREE.AdditiveBlending : THREE.NormalBlending,
      uniforms: {
        uTime: { value: 0 },
        uSize: { value: 26 * renderer.getPixelRatio() },
        uCamZ: { value: 3.6 },
        uPing: { value: pings },
        uPingT: { value: pingT },
        uBase: { value: new THREE.Color(dark ? "#3d93df" : "#4a8cc9") },
        uHot: { value: new THREE.Color(dark ? "#e4f4ff" : "#38a4ff") },
      },
    });
    world.add(new THREE.Points(geo, mat));

    // atmosphere: a back-faced shell that glows at the rim (fresnel), so the globe reads as lit
    const atmoGeo = new THREE.SphereGeometry(1.22, 64, 64);
    const atmoMat = new THREE.ShaderMaterial({
      side: THREE.BackSide,
      transparent: true,
      depthWrite: false,
      blending: dark ? THREE.AdditiveBlending : THREE.NormalBlending,
      uniforms: { uColor: { value: new THREE.Color(dark ? "#2fa8ff" : "#0071d1") }, uPower: { value: dark ? 0.55 : 0.28 } },
      vertexShader: /* glsl */ `
        varying vec3 vNormal;
        void main() {
          vNormal = normalize(normalMatrix * normal);
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        }`,
      fragmentShader: /* glsl */ `
        uniform vec3 uColor;
        uniform float uPower;
        varying vec3 vNormal;
        void main() {
          // d: 0 at the shell's outer edge, ~0.57 where the globe's own edge is; peak there, fade both ways
          float d = clamp(-dot(vNormal, vec3(0.0, 0.0, 1.0)), 0.0, 1.0);
          float rim = exp(-pow((d - 0.5) * 7.0, 2.0));
          gl_FragColor = vec4(uColor, rim * uPower);
        }`,
    });
    world.add(new THREE.Mesh(atmoGeo, atmoMat));

    // an orbiting band of dust, tilted: the auction floor going round
    const ringN = Math.round(density / 4);
    const ringPos = new Float32Array(ringN * 3);
    for (let i = 0; i < ringN; i++) {
      const a = Math.random() * Math.PI * 2;
      const r = 1.42 + (Math.random() - 0.5) * 0.18 + Math.random() ** 3 * 0.35;
      ringPos.set([Math.cos(a) * r, (Math.random() - 0.5) * 0.03, Math.sin(a) * r], i * 3);
    }
    const ringGeo = new THREE.BufferGeometry();
    ringGeo.setAttribute("position", new THREE.BufferAttribute(ringPos, 3));
    const ringMat = new THREE.PointsMaterial({
      size: 0.012,
      color: dark ? "#4aa3ea" : "#4a8cc9",
      transparent: true,
      opacity: dark ? 0.55 : 0.5,
      depthWrite: false,
      blending: dark ? THREE.AdditiveBlending : THREE.NormalBlending,
    });
    const ring = new THREE.Points(ringGeo, ringMat);
    ring.rotation.set(1.15, 0, 0.32);
    world.add(ring);

    // name tags that follow their ping across the screen
    type Tag = { el: HTMLDivElement; at: THREE.Vector3; born: number };
    const tags: Tag[] = [];
    const spot = (t: number, slot: number) => {
      // pick a point on the side facing the camera so the tag is visible
      const local = new THREE.Vector3();
      for (let i = 0; i < 30; i++) {
        local.randomDirection();
        if (local.clone().applyQuaternion(world.quaternion).z > 0.25) break;
      }
      pings[slot].copy(local);
      pingT[slot] = t;
      const name = NAMES[Math.floor(Math.random() * NAMES.length)];
      const price = Math.round(8 + Math.random() ** 2.4 * 900);
      const tag = document.createElement("div");
      tag.className =
        "pointer-events-none absolute left-0 top-0 flex items-center gap-2 whitespace-nowrap rounded-full border border-primary/40 bg-background/70 px-2.5 py-1 text-[12px] backdrop-blur-sm transition-opacity duration-500";
      tag.innerHTML = `<span class="size-1.5 rounded-full bg-primary"></span><span class="font-medium text-foreground">@${name}</span><span class="data text-muted-foreground">${price} TON</span>`;
      tag.style.opacity = "0";
      tagLayer.appendChild(tag);
      tags.push({ el: tag, at: local.clone(), born: t });
    };

    // the globe leans toward the pointer
    const target = { x: 0, y: 0 };
    const onMove = (e: PointerEvent) => {
      const r = el.getBoundingClientRect();
      target.x = ((e.clientX - r.left) / r.width - 0.5) * 2;
      target.y = ((e.clientY - r.top) / r.height - 0.5) * 2;
    };
    window.addEventListener("pointermove", onMove, { passive: true });

    const resize = () => {
      const { width, height } = el.getBoundingClientRect();
      if (!width || !height) return;
      renderer.setSize(width, height, false);
      camera.aspect = width / height;
      // frame the whole ring (diameter ~3.3) inside 92% of the width and 62% of the height,
      // then lift it so the copy below has room
      const tan = Math.tan(THREE.MathUtils.degToRad(camera.fov / 2));
      const short = height <= 320; // the mobile strip: no copy to make room for
      const z = Math.max(3.3 / 0.92 / (2 * tan * camera.aspect), 3.3 / (short ? 1.0 : 0.58) / (2 * tan));
      camera.position.z = z;
      mat.uniforms.uCamZ.value = z;
      // point sizes are in pixels: shrink them with the globe so a small one doesn't white out
      const globePx = (2.2 / (2 * z * tan)) * height;
      mat.uniforms.uSize.value = 26 * renderer.getPixelRatio() * THREE.MathUtils.clamp(globePx / 480, 0.35, 1.25);
      world.position.y = short ? 0 : 2 * z * tan * 0.09;
      camera.updateProjectionMatrix();
    };
    const ro = new ResizeObserver(resize);
    ro.observe(el);
    resize();

    const clock = new THREE.Clock();
    let nextPing = 0.4;
    let slot = 0;
    let raf = 0;
    const v = new THREE.Vector3();
    const frame = () => {
      const t = clock.getElapsedTime();
      const speed = reduced ? 0.15 : 1;
      mat.uniforms.uTime.value = t * speed;
      world.rotation.y += 0.0016 * speed;
      world.rotation.x += (0.35 + target.y * 0.18 - world.rotation.x) * 0.04;
      world.rotation.z += (-target.x * 0.12 - world.rotation.z) * 0.04;
      ring.rotation.y -= 0.0022 * speed;

      if (!reduced && t > nextPing) {
        spot(t * speed, slot);
        slot = (slot + 1) % PINGS;
        nextPing = t + PING_EVERY * (0.7 + Math.random() * 0.6);
      }

      const { width, height } = el.getBoundingClientRect();
      for (let i = tags.length - 1; i >= 0; i--) {
        const tag = tags[i];
        const age = t * speed - tag.born;
        if (age > PING_LIFE) {
          tag.el.remove();
          tags.splice(i, 1);
          continue;
        }
        v.copy(tag.at).multiplyScalar(1.08).applyMatrix4(world.matrixWorld);
        const behind = v.clone().sub(camera.position).dot(v) > 0; // on the far side of the globe
        v.project(camera);
        const x = (v.x * 0.5 + 0.5) * width;
        const y = (-v.y * 0.5 + 0.5) * height - 18 - age * 10;
        tag.el.style.transform = `translate(${x + 12}px, ${y}px)`;
        tag.el.style.opacity = behind || age > PING_LIFE - 0.8 ? "0" : "1";
      }

      renderer.render(scene, camera);
      raf = requestAnimationFrame(frame);
    };
    frame();

    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
      window.removeEventListener("pointermove", onMove);
      tags.forEach((t) => t.el.remove());
      geo.dispose();
      mat.dispose();
      ringGeo.dispose();
      ringMat.dispose();
      atmoGeo.dispose();
      atmoMat.dispose();
      renderer.dispose();
      renderer.domElement.remove();
    };
  }, [density]);

  return (
    <div ref={host} className={className} aria-hidden>
      <div ref={labels} className="absolute inset-0 overflow-hidden" />
    </div>
  );
}
