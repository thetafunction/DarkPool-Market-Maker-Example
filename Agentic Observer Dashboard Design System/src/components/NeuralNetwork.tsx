import React, { useMemo, useRef } from 'react';
import { Canvas, useFrame } from '@react-three/fiber';
import { Html, Line, OrbitControls, Sparkles, Stars } from '@react-three/drei';
import * as THREE from 'three';

type NeuralNode = {
  id: string;
  label: string;
  tier: 'alpha' | 'signal' | 'settlement';
  position: [number, number, number];
  throughput: number;
};

type NeuralLink = {
  from: string;
  to: string;
  value: number;
  speed: number;
  phase: number;
};

const NODES: NeuralNode[] = [
  { id: 'CORE', label: 'Deluthium Core', tier: 'alpha', position: [0, 0, 0], throughput: 99 },
  { id: 'A1', label: 'Quant Agent', tier: 'signal', position: [-3.6, 1.9, 1.4], throughput: 93 },
  { id: 'A2', label: 'Macro Agent', tier: 'signal', position: [3.2, 2.2, -1.8], throughput: 89 },
  { id: 'A3', label: 'Arb Agent', tier: 'signal', position: [2.8, -2.6, 2.2], throughput: 96 },
  { id: 'A4', label: 'News Agent', tier: 'signal', position: [-3.1, -2.1, -2.4], throughput: 87 },
  { id: 'S1', label: 'Settlement L2', tier: 'settlement', position: [5.2, 0.2, 0.3], throughput: 91 },
  { id: 'S2', label: 'Liquidity Vault', tier: 'settlement', position: [-5.1, -0.6, -0.4], throughput: 94 },
];

const LINKS: NeuralLink[] = [
  { from: 'CORE', to: 'A1', value: 78, speed: 0.24, phase: 0.0 },
  { from: 'CORE', to: 'A2', value: 83, speed: 0.21, phase: 0.18 },
  { from: 'CORE', to: 'A3', value: 92, speed: 0.26, phase: 0.35 },
  { from: 'CORE', to: 'A4', value: 74, speed: 0.22, phase: 0.52 },
  { from: 'A1', to: 'S2', value: 68, speed: 0.3, phase: 0.1 },
  { from: 'A2', to: 'S1', value: 76, speed: 0.31, phase: 0.25 },
  { from: 'A3', to: 'S1', value: 88, speed: 0.35, phase: 0.4 },
  { from: 'A4', to: 'S2', value: 71, speed: 0.3, phase: 0.57 },
];

function tierColor(tier: NeuralNode['tier']) {
  if (tier === 'alpha') return '#00f5ff';
  if (tier === 'signal') return '#9f7bff';
  return '#00ff9d';
}

const linkColor = '#3de8ff';

const Packet: React.FC<{ start: THREE.Vector3; end: THREE.Vector3; speed: number; phase: number }> = ({
  start,
  end,
  speed,
  phase,
}) => {
  const ref = useRef<THREE.Mesh>(null);

  useFrame(({ clock }) => {
    if (!ref.current) return;
    const t = (clock.elapsedTime * speed + phase) % 1;
    ref.current.position.lerpVectors(start, end, t);
    const pulse = 0.07 + Math.sin(clock.elapsedTime * 8 + phase * 10) * 0.015;
    ref.current.scale.setScalar(pulse);
  });

  return (
    <mesh ref={ref}>
      <sphereGeometry args={[1, 12, 12]} />
      <meshBasicMaterial color="#d4f8ff" />
    </mesh>
  );
};

const NetworkScene: React.FC = () => {
  const nodeMap = useMemo(() => {
    const map = new Map<string, NeuralNode>();
    NODES.forEach((n) => map.set(n.id, n));
    return map;
  }, []);

  return (
    <>
      <color attach="background" args={['#050913']} />
      <fog attach="fog" args={['#050913', 8, 24]} />
      <ambientLight intensity={0.5} />
      <pointLight position={[0, 0, 0]} intensity={2.5} color="#0fffff" />
      <pointLight position={[5, 4, -2]} intensity={1.2} color="#7d69ff" />

      <Stars radius={40} depth={45} count={1800} factor={2} fade speed={1.2} />
      <Sparkles count={80} size={2} speed={0.35} scale={[14, 8, 14]} color="#57e6ff" />

      {LINKS.map((link) => {
        const from = nodeMap.get(link.from);
        const to = nodeMap.get(link.to);
        if (!from || !to) return null;

        const start = new THREE.Vector3(...from.position);
        const end = new THREE.Vector3(...to.position);

        return (
          <group key={`${link.from}-${link.to}`}>
            <Line points={[from.position, to.position]} color={linkColor} transparent opacity={0.45} lineWidth={1.2} />
            <Packet start={start} end={end} speed={link.speed} phase={link.phase} />
          </group>
        );
      })}

      {NODES.map((node) => (
        <group key={node.id} position={node.position}>
          <mesh>
            <sphereGeometry args={[node.id === 'CORE' ? 0.42 : 0.27, 24, 24]} />
            <meshStandardMaterial
              emissive={new THREE.Color(tierColor(node.tier))}
              emissiveIntensity={0.9}
              color={node.id === 'CORE' ? '#10243a' : '#120f2d'}
              roughness={0.28}
              metalness={0.15}
            />
          </mesh>

          <mesh>
            <sphereGeometry args={[node.id === 'CORE' ? 0.75 : 0.45, 20, 20]} />
            <meshBasicMaterial color={tierColor(node.tier)} transparent opacity={0.09} />
          </mesh>

          <Html center distanceFactor={10} position={[0, node.id === 'CORE' ? -0.95 : -0.65, 0]}>
            <div
              style={{
                whiteSpace: 'nowrap',
                fontSize: '11px',
                letterSpacing: '0.08em',
                color: '#aeefff',
                textShadow: '0 0 8px rgba(0,240,255,0.7)',
                textTransform: 'uppercase',
                fontFamily: 'Courier New, monospace',
              }}
            >
              {node.label} | {node.throughput}% throughput
            </div>
          </Html>
        </group>
      ))}

      <OrbitControls enablePan={false} minDistance={8} maxDistance={16} autoRotate autoRotateSpeed={0.35} />
    </>
  );
};

export const NeuralNetwork: React.FC = () => {
  return (
    <div
      className="pipboy-box p-4 h-full flex flex-col"
      style={{ borderColor: '#1e5a79', boxShadow: '0 0 18px rgba(61,232,255,0.35)' }}
    >
      <div className="mb-3 flex items-center justify-between">
        <div className="text-sm pixel-text" style={{ color: '#82ecff', textShadow: '0 0 10px rgba(0,234,255,0.8)' }}>
          NEURAL MARKETPLACE GRAPH
        </div>
        <div className="text-xs dim-text">LIVING AGENT ECONOMY</div>
      </div>

      <div className="flex-1 relative overflow-hidden" style={{ minHeight: 320 }}>
        <Canvas camera={{ position: [0, 2.2, 11], fov: 48 }}>
          <NetworkScene />
        </Canvas>
      </div>

      <div className="mt-3 text-xs flex justify-between" style={{ color: '#73d9f4' }}>
        <span>Active Links: {LINKS.length}</span>
        <span>Node Uptime: 99.97%</span>
        <span>Latency: 42ms</span>
      </div>
    </div>
  );
};
