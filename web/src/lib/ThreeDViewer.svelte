<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import * as THREE from "three";
  import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
  import { GLTFLoader } from "three/examples/jsm/loaders/GLTFLoader.js";
  import { OBJLoader } from "three/examples/jsm/loaders/OBJLoader.js";
  import { PLYLoader } from "three/examples/jsm/loaders/PLYLoader.js";
  import { STLLoader } from "three/examples/jsm/loaders/STLLoader.js";

  export let src: string;
  export let name = "";

  let container: HTMLDivElement;
  let canvas: HTMLCanvasElement;
  let renderer: THREE.WebGLRenderer | null = null;
  let scene: THREE.Scene;
  let camera: THREE.PerspectiveCamera;
  let controls: OrbitControls;
  let resizeObserver: ResizeObserver | null = null;
  let frame = 0;
  let currentObject: THREE.Object3D | null = null;
  let mounted = false;
  let loading = true;
  let error = "";
  let loadedSrc = "";

  $: if (mounted && src && src !== loadedSrc) {
    loadModel(src);
  }

  onMount(() => {
    scene = new THREE.Scene();
    scene.background = new THREE.Color("#111820");

    camera = new THREE.PerspectiveCamera(45, 1, 0.01, 10000);
    camera.position.set(2.5, 1.8, 3);

    renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    renderer.toneMapping = THREE.ACESFilmicToneMapping;
    renderer.toneMappingExposure = 1.1;

    controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.dampingFactor = 0.08;
    controls.screenSpacePanning = true;

    const hemisphere = new THREE.HemisphereLight("#ffffff", "#5d6a74", 2.2);
    scene.add(hemisphere);

    const keyLight = new THREE.DirectionalLight("#ffffff", 2.8);
    keyLight.position.set(3, 4, 5);
    scene.add(keyLight);

    const fillLight = new THREE.DirectionalLight("#d7edf7", 1.2);
    fillLight.position.set(-4, 2, -3);
    scene.add(fillLight);

    const grid = new THREE.GridHelper(10, 20, "#33404a", "#24313a");
    grid.position.y = -0.02;
    scene.add(grid);

    resizeObserver = new ResizeObserver(resize);
    resizeObserver.observe(container);
    resize();
    mounted = true;
    loadModel(src);
    animate();
  });

  onDestroy(cleanup);

  function cleanup() {
    mounted = false;
    if (frame) cancelAnimationFrame(frame);
    frame = 0;
    resizeObserver?.disconnect();
    resizeObserver = null;
    disposeCurrentObject();
    controls?.dispose();
    renderer?.dispose();
    renderer = null;
  }

  function animate() {
    if (!renderer) return;
    frame = requestAnimationFrame(animate);
    controls.update();
    renderer.render(scene, camera);
  }

  function resize() {
    if (!renderer || !container) return;
    const width = Math.max(container.clientWidth, 1);
    const height = Math.max(container.clientHeight, 1);
    camera.aspect = width / height;
    camera.updateProjectionMatrix();
    renderer.setSize(width, height, false);
  }

  function loadModel(url: string) {
    if (!url) return;
    loadedSrc = url;
    loading = true;
    error = "";
    disposeCurrentObject();

    const extension = modelExtension(name || url);
    const onError = () => {
      if (loadedSrc !== url) return;
      loading = false;
      error = extension === "gltf"
        ? "This GLTF could not be loaded. Use GLB or a self-contained GLTF bundle."
        : "This 3D file could not be loaded in the browser.";
    };
    const onObject = (object: THREE.Object3D) => {
      if (loadedSrc !== url) {
        disposeObject(object);
        return;
      }
      currentObject = object;
      scene.add(object);
      fitObject(object);
      loading = false;
    };

    if (extension === "glb" || extension === "gltf") {
      new GLTFLoader().load(url, result => onObject(result.scene), undefined, onError);
      return;
    }
    if (extension === "stl") {
      new STLLoader().load(url, geometry => {
        geometry.computeVertexNormals();
        onObject(new THREE.Mesh(geometry, defaultMaterial()));
      }, undefined, onError);
      return;
    }
    if (extension === "obj") {
      new OBJLoader().load(url, onObject, undefined, onError);
      return;
    }
    if (extension === "ply") {
      new PLYLoader().load(url, geometry => {
        geometry.computeVertexNormals();
        onObject(new THREE.Mesh(geometry, defaultMaterial()));
      }, undefined, onError);
      return;
    }

    loading = false;
    error = "This 3D format is not supported by the browser preview.";
  }

  function modelExtension(value: string) {
    const clean = value.split("?")[0].split("#")[0].toLowerCase();
    const dot = clean.lastIndexOf(".");
    return dot >= 0 ? clean.slice(dot + 1) : "";
  }

  function defaultMaterial() {
    return new THREE.MeshStandardMaterial({
      color: "#d9dee2",
      roughness: 0.58,
      metalness: 0.08
    });
  }

  function fitObject(object: THREE.Object3D) {
    const box = new THREE.Box3().setFromObject(object);
    if (box.isEmpty()) return;

    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    object.position.sub(center);

    const maxDim = Math.max(size.x, size.y, size.z, 1);
    const distance = maxDim / (2 * Math.tan(THREE.MathUtils.degToRad(camera.fov) / 2));
    camera.near = Math.max(maxDim / 1000, 0.001);
    camera.far = Math.max(maxDim * 100, 100);
    camera.position.set(distance * 0.9, distance * 0.65, distance * 1.35);
    camera.updateProjectionMatrix();
    controls.target.set(0, 0, 0);
    controls.update();
  }

  function disposeCurrentObject() {
    if (!currentObject) return;
    scene.remove(currentObject);
    disposeObject(currentObject);
    currentObject = null;
  }

  function disposeObject(object: THREE.Object3D) {
    object.traverse(node => {
      const mesh = node as THREE.Mesh;
      if (mesh.geometry) mesh.geometry.dispose();
      const material = mesh.material;
      if (Array.isArray(material)) {
        material.forEach(disposeMaterial);
      } else if (material) {
        disposeMaterial(material);
      }
    });
  }

  function disposeMaterial(material: THREE.Material) {
    for (const value of Object.values(material)) {
      if (value && typeof value === "object" && "isTexture" in value) {
        (value as THREE.Texture).dispose();
      }
    }
    material.dispose();
  }
</script>

<div class="model-viewer" bind:this={container}>
  <canvas bind:this={canvas} aria-label={name}></canvas>
  {#if loading}
    <div class="model-viewer-overlay">Loading 3D model</div>
  {:else if error}
    <div class="model-viewer-overlay error">{error}</div>
  {/if}
</div>

<style>
  .model-viewer {
    position: relative;
    width: 100%;
    height: 100%;
    min-height: 360px;
    overflow: hidden;
    border-radius: 6px;
    background: #111820;
  }

  canvas {
    display: block;
    width: 100%;
    height: 100%;
    touch-action: none;
  }

  .model-viewer-overlay {
    position: absolute;
    left: 50%;
    bottom: 18px;
    max-width: min(520px, calc(100% - 32px));
    transform: translateX(-50%);
    padding: 8px 12px;
    border-radius: 4px;
    background: rgba(10, 16, 22, 0.82);
    color: #f4f7f8;
    font-size: 13px;
    line-height: 1.35;
    text-align: center;
    pointer-events: none;
  }

  .model-viewer-overlay.error {
    background: rgba(93, 25, 25, 0.9);
  }
</style>
