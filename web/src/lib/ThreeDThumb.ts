/**
 * Singleton 3D thumbnail renderer.
 * Uses one hidden canvas + Three.js to render 3D models on demand.
 * Results are cached by file path and returned as data-URL strings.
 */
import * as THREE from "three";
import { GLTFLoader } from "three/examples/jsm/loaders/GLTFLoader.js";
import { OBJLoader } from "three/examples/jsm/loaders/OBJLoader.js";
import { PLYLoader } from "three/examples/jsm/loaders/PLYLoader.js";
import { STLLoader } from "three/examples/jsm/loaders/STLLoader.js";
import { ThreeMFLoader } from "three/examples/jsm/loaders/3MFLoader.js";

const THUMB_SIZE = 240;
const cache = new Map<string, string | null>(); // null = failed
const pending = new Map<string, Array<(url: string | null) => void>>();

let renderer: THREE.WebGLRenderer | null = null;
let scene: THREE.Scene | null = null;
let camera: THREE.PerspectiveCamera | null = null;

function init() {
  if (renderer) return;
  const canvas = document.createElement("canvas");
  canvas.width = THUMB_SIZE;
  canvas.height = THUMB_SIZE;
  renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: false });
  renderer.setSize(THUMB_SIZE, THUMB_SIZE, false);
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.toneMapping = THREE.ACESFilmicToneMapping;

  scene = new THREE.Scene();
  scene.background = new THREE.Color("#1a2028");

  camera = new THREE.PerspectiveCamera(45, 1, 0.001, 100000);

  const hemi = new THREE.HemisphereLight("#ffffff", "#5d6a74", 2.2);
  scene.add(hemi);
  const key = new THREE.DirectionalLight("#ffffff", 2.8);
  key.position.set(3, 4, 5);
  scene.add(key);
  const fill = new THREE.DirectionalLight("#d7edf7", 1.2);
  fill.position.set(-4, 2, -3);
  scene.add(fill);
}

function ext(name: string) {
  const clean = name.split("?")[0].toLowerCase();
  const dot = clean.lastIndexOf(".");
  return dot >= 0 ? clean.slice(dot + 1) : "";
}

function defaultMat() {
  return new THREE.MeshStandardMaterial({ color: "#d9dee2", roughness: 0.58, metalness: 0.08 });
}

function fit(object: THREE.Object3D) {
  const box = new THREE.Box3().setFromObject(object);
  if (box.isEmpty() || !camera) return;
  const center = box.getCenter(new THREE.Vector3());
  const size = box.getSize(new THREE.Vector3());
  object.position.sub(center);
  const maxDim = Math.max(size.x, size.y, size.z, 0.001);
  const dist = maxDim / (2 * Math.tan(THREE.MathUtils.degToRad(22.5)));
  camera.near = Math.max(maxDim / 1000, 0.001);
  camera.far = Math.max(maxDim * 100, 100);
  camera.position.set(dist * 0.9, dist * 0.65, dist * 1.35);
  camera.updateProjectionMatrix();
  camera.lookAt(0, 0, 0);
}

function renderObject(object: THREE.Object3D): string {
  if (!renderer || !scene || !camera) return "";
  fit(object);
  scene.add(object);
  renderer.render(scene, camera);
  const url = renderer.domElement.toDataURL("image/jpeg", 0.85);
  scene.remove(object);
  disposeObject(object);
  return url;
}

function disposeObject(obj: THREE.Object3D) {
  obj.traverse(node => {
    const m = node as THREE.Mesh;
    m.geometry?.dispose();
    const mat = m.material;
    if (Array.isArray(mat)) mat.forEach(x => x.dispose());
    else (mat as THREE.Material)?.dispose();
  });
}

function resolve(path: string, result: string | null) {
  cache.set(path, result);
  pending.get(path)?.forEach(cb => cb(result));
  pending.delete(path);
}

async function renderURL(src: string, name: string): Promise<string | null> {
  init();
  const e = ext(name || src);
  return new Promise(ok => {
    const onErr = () => ok(null);
    const onObj = (obj: THREE.Object3D) => {
      try { ok(renderObject(obj)); } catch { ok(null); }
    };
    if (e === "glb" || e === "gltf") {
      new GLTFLoader().load(src, r => onObj(r.scene), undefined, onErr);
    } else if (e === "stl") {
      new STLLoader().load(src, geo => {
        geo.computeVertexNormals();
        onObj(new THREE.Mesh(geo, defaultMat()));
      }, undefined, onErr);
    } else if (e === "obj") {
      new OBJLoader().load(src, onObj, undefined, onErr);
    } else if (e === "ply") {
      new PLYLoader().load(src, geo => {
        geo.computeVertexNormals();
        onObj(new THREE.Mesh(geo, defaultMat()));
      }, undefined, onErr);
    } else if (e === "3mf") {
      new ThreeMFLoader().load(src, onObj, undefined, onErr);
    } else {
      ok(null);
    }
  });
}

export async function getThumb(path: string, src: string, name: string): Promise<string | null> {
  if (cache.has(path)) return cache.get(path) ?? null;

  if (pending.has(path)) {
    return new Promise(cb => pending.get(path)!.push(cb));
  }

  pending.set(path, []);
  const result = await renderURL(src, name);
  resolve(path, result);
  return result;
}
