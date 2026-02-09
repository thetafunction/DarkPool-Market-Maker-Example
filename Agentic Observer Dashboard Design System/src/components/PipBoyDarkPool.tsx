import React, { useState, useEffect } from 'react';
import { motion } from 'motion/react';

interface Agent {
  id: number;
  x: number;
  y: number;
  phase: 'entering' | 'inside' | 'exiting';
}

export const PipBoyDarkPool: React.FC = () => {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [activeAgents, setActiveAgents] = useState(234);
  const [operations, setOperations] = useState(1247);

  useEffect(() => {
    // Generate agents continuously
    const interval = setInterval(() => {
      setAgents(prev => {
        const newAgent: Agent = {
          id: Date.now() + Math.random(),
          x: -20,
          y: 50 + (Math.random() - 0.5) * 40,
          phase: 'entering'
        };
        
        // Update existing agents and add new one
        const updated = prev.map(agent => {
          if (agent.x < 30) return { ...agent, x: agent.x + 2, phase: 'entering' as const };
          if (agent.x < 70) return { ...agent, x: agent.x + 0.5, phase: 'inside' as const };
          return { ...agent, x: agent.x + 2, phase: 'exiting' as const };
        }).filter(agent => agent.x < 120);

        return [...updated, newAgent];
      });

      setActiveAgents(prev => prev + Math.floor(Math.random() * 3 - 1));
      setOperations(prev => prev + Math.floor(Math.random() * 10));
    }, 800);

    return () => clearInterval(interval);
  }, []);

  return (
    <div className="pipboy-box p-6 h-full flex flex-col">
      {/* Title */}
      <div className="text-center mb-4">
        <div className="text-sm radioactive-text pixel-text mb-2">
          SHIELDED EXECUTION MATRIX
        </div>
        <div className="text-xs dim-text">
          SHIELDED EXECUTION SYSTEM
        </div>
      </div>

      {/* Main visualization area */}
      <div className="flex-1 relative overflow-hidden">
        {/* Rotating wireframe sphere */}
        <div className="absolute inset-0 flex items-center justify-center">
          <svg
            viewBox="0 0 400 400"
            className="w-full h-full max-w-md animate-rotate-3d"
            style={{ 
              filter: 'drop-shadow(0 0 10px rgba(57, 255, 20, 0.5))',
              transformStyle: 'preserve-3d'
            }}
          >
            {/* Wireframe sphere - latitude lines */}
            {Array.from({ length: 9 }).map((_, i) => {
              const y = 50 + i * 37.5;
              const radiusScale = Math.sin((i / 8) * Math.PI);
              const width = 300 * radiusScale;
              
              return (
                <ellipse
                  key={`lat-${i}`}
                  cx="200"
                  cy={y}
                  rx={width / 2}
                  ry="15"
                  fill="none"
                  stroke="#008F11"
                  strokeWidth="1"
                  opacity={0.3 + radiusScale * 0.3}
                />
              );
            })}

            {/* Wireframe sphere - longitude lines */}
            {Array.from({ length: 12 }).map((_, i) => {
              const angle = (i * 30) * Math.PI / 180;
              return (
                <ellipse
                  key={`lon-${i}`}
                  cx="200"
                  cy="200"
                  rx="150"
                  ry="150"
                  fill="none"
                  stroke="#008F11"
                  strokeWidth="1"
                  opacity="0.4"
                  transform={`rotate(${i * 15} 200 200)`}
                />
              );
            })}

            {/* Center shielded core */}
            <circle
              cx="200"
              cy="200"
              r="80"
              fill="rgba(57, 255, 20, 0.05)"
              stroke="#39FF14"
              strokeWidth="2"
              className="animate-glow-pulse"
            />
            
            <circle
              cx="200"
              cy="200"
              r="60"
              fill="none"
              stroke="#39FF14"
              strokeWidth="1"
              opacity="0.5"
            />

            {/* Center lock icon */}
            <text
              x="200"
              y="215"
              textAnchor="middle"
              fill="#39FF14"
              fontSize="40"
              className="animate-glow-pulse"
            >
              🔒
            </text>
          </svg>
        </div>

        {/* Agent particles */}
        <svg viewBox="0 0 100 100" className="absolute inset-0 w-full h-full">
          {agents.map(agent => (
            <motion.circle
              key={agent.id}
              cx={agent.x}
              cy={agent.y}
              r={agent.phase === 'inside' ? 0.5 : 1}
              fill="#39FF14"
              initial={{ opacity: 0 }}
              animate={{ 
                opacity: agent.phase === 'inside' ? 0.3 : 1,
              }}
              style={{
                filter: 'drop-shadow(0 0 3px #39FF14)',
              }}
            />
          ))}
        </svg>

        {/* Entry/Exit labels */}
        <div className="absolute left-4 top-1/2 -translate-y-1/2 text-xs dim-text pixel-text">
          ENTRY →
        </div>
        <div className="absolute right-4 top-1/2 -translate-y-1/2 text-xs dim-text pixel-text">
          ← EXIT
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-2 mt-4">
        <div className="pipboy-box p-2 text-center bg-[#000000]">
          <div className="text-xs dim-text pixel-text mb-1">AGENTS</div>
          <div className="text-lg radioactive-text pixel-text">{activeAgents}</div>
        </div>
        <div className="pipboy-box p-2 text-center bg-[#000000]">
          <div className="text-xs dim-text pixel-text mb-1">TEE_OPS</div>
          <div className="text-lg radioactive-text pixel-text">{operations}</div>
        </div>
        <div className="pipboy-box p-2 text-center bg-[#000000]">
          <div className="text-xs dim-text pixel-text mb-1">EXPOSED</div>
          <div className="text-lg radioactive-text pixel-text">0</div>
        </div>
      </div>

      {/* Bottom label */}
      <div className="mt-4 text-center text-[10px] dim-text italic">
        &gt;&gt; THE INVISIBLE HAND IN ACTION
      </div>
    </div>
  );
};
