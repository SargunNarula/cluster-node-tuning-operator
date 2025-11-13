# MCP Server Value Proposition: Scenario Comparison

Exhaustive list of scenarios showing the difference between responses **with** and **without** the MCP server.

---

## Category 1: Natural Language Understanding

### Scenario 1.1: Vague Workload Description

**User Prompt:**
> "I need to tune my cluster for telecom workloads"

**WITHOUT MCP:**
```
I can help you create a performance profile. You'll need to:
1. Run must-gather to collect cluster data
2. Determine your CPU allocation (isolated vs reserved)
3. Decide if you need RT kernel
4. Configure the PPC command with appropriate flags
5. Run the PPC tool

Here's a generic example:
podman run ... performance-profile-creator \
  --mcp-name worker-cnf \
  --reserved-cpu-count 4 \
  --rt-kernel true \
  ...

You'll need to figure out the exact parameters based on your requirements.
```

**WITH MCP:**
```
Let me help you with that! I've identified this as a Telco VNF workload.

Based on your requirements, I recommend the "Telco VNF" template:

📋 Workload Analysis:
- Type: Telco VNF (Virtual Network Function)
- Use Cases: Virtual routers, Session border controllers, Packet gateways, DPI
- Requires: RT kernel ✓, DPDK ✓, Low-latency power mode

🔧 Recommended Configuration:
- RT Kernel: Enabled (for deterministic latency)
- DPDK: Enabled (for high-performance packet processing)
- Hyperthreading: Keep enabled (unless you need absolute determinism)
- Power Mode: low-latency
- Topology Policy: restricted (NUMA-aware)

📊 CPU Allocation (adjust based on your must-gather):
- Reserved CPUs: 4-8 (for system/kubelet)
- Isolated CPUs: Remaining cores (for VNF workloads)
- Hugepages: 1G recommended

Would you like me to:
1. Analyze your cluster hardware (provide must-gather path)
2. Generate the exact PPC command
3. Validate if your hardware supports these settings
```

---

### Scenario 1.2: Complex Multi-Requirement

**User Prompt:**
> "I need ultra-low latency, my app processes real-time 5G traffic, needs dedicated CPUs, and I heard something about hugepages"

**WITHOUT MCP:**
```
That's a complex setup. You'll need:
- Enable RT kernel (--rt-kernel true)
- Set up CPU isolation (--isolated-cpu-count X)
- Configure hugepages (separate configuration)
- Maybe disable hyperthreading?
- Power management settings

I recommend reading the PPC documentation to understand all the flags.
Check must-gather output to see your CPU topology.
Then construct the PPC command carefully.
```

**WITH MCP:**
```
Perfect! Your requirements match our "5G RAN (Radio Access Network)" template.

🎯 Requirement Analysis:
✓ Ultra-low latency → RT kernel + ultra-low-latency power mode
✓ Real-time 5G traffic → DPDK for packet processing
✓ Dedicated CPUs → CPU isolation + disable hyperthreading
✓ Hugepages → 1G hugepages for DPDK

🔧 Generated Configuration:
- RT Kernel: ✓ Enabled (PREEMPT_RT for microsecond latency)
- Hyperthreading: ✗ Disabled (for deterministic performance)
- DPDK: ✓ Enabled (user-level packet processing)
- Power Mode: ultra-low-latency (C-states disabled, max frequency)
- Topology Policy: single-numa-node (strict NUMA placement)
- Hugepages: 1G (allocate based on your memory needs)

⚠️ Important Considerations:
1. Disabling HT reduces total compute by ~30-40%
2. Ultra-low-latency power mode increases power consumption
3. RT kernel may impact non-real-time workloads
4. Ensure your NICs support DPDK

📝 Next Step:
Provide your must-gather path, and I'll:
1. Validate your hardware supports these settings
2. Calculate optimal CPU allocation
3. Generate the exact PPC command
```

---

## Category 2: Hardware Validation

### Scenario 2.1: Requesting Too Many CPUs

**User Prompt:**
> "Generate a profile with 60 isolated CPUs and 4 reserved CPUs"

**WITHOUT MCP:**
```
Here's the command:

podman run ... performance-profile-creator \
  --mcp-name worker-cnf \
  --reserved-cpu-count 4 \
  --isolated-cpu-count 60 \
  --rt-kernel true \
  --must-gather-dir-path /must-gather \
  > profile.yaml
```
*User runs it, PPC fails or creates invalid profile*

**WITH MCP:**
```
⚠️ Hardware Validation Failed!

I analyzed your cluster hardware and found:
- Total CPUs per node: 48 (24 physical cores × 2 with HT)
- Available for allocation: 48 CPUs
- You requested: 64 CPUs total (60 isolated + 4 reserved)

❌ Problem: You're requesting 16 more CPUs than available!

💡 Recommendations:
1. With Hyperthreading (48 CPUs total):
   - Reserved: 4-8 CPUs (for system/kubelet)
   - Isolated: 40-44 CPUs (for workloads)

2. Without Hyperthreading (24 physical cores):
   - Reserved: 4 CPUs
   - Isolated: 20 CPUs
   Note: This reduces total compute but improves determinism

3. Multi-Node Strategy:
   If you need 60 CPUs per workload, consider:
   - Using nodes with more cores (e.g., dual-socket servers)
   - Spreading workload across multiple nodes
   - Requesting hardware upgrade

Would you like me to generate a profile with corrected CPU allocation?
```

---

### Scenario 2.2: Incompatible Hardware Features

**User Prompt:**
> "Enable DPDK and use 1G hugepages on my cluster"

**WITHOUT MCP:**
```
Here's the command:

podman run ... performance-profile-creator \
  --user-level-networking true \
  --hugepage-size 1G \
  --must-gather-dir-path /must-gather \
  > profile.yaml
```
*User applies it, workload fails because hardware doesn't support 1G hugepages*

**WITH MCP:**
```
⚠️ Hardware Compatibility Check:

I analyzed your cluster and found potential issues:

❌ 1G Hugepages: NOT SUPPORTED
   - Your CPU architecture: x86_64
   - Kernel version: 4.18.0
   - Issue: Kernel not compiled with 1G hugepage support
   - Evidence: /sys/kernel/mm/hugepages/hugepages-1048576kB not found

✅ DPDK: SUPPORTED
   - NICs detected: Intel X710 (DPDK compatible)
   - IOMMU: Enabled
   - Hugepages (2M): Supported

💡 Recommended Fix:
Use 2M hugepages instead:

podman run ... performance-profile-creator \
  --user-level-networking true \
  --hugepage-size 2M \
  --hugepage-count 8192 \  # 16GB total (adjust as needed)
  --must-gather-dir-path /must-gather

📊 Hugepage Calculation (2M pages):
- 2048 pages = 4GB
- 4096 pages = 8GB
- 8192 pages = 16GB
- 16384 pages = 32GB

How much memory does your DPDK application need?
```

---

## Category 3: Template & Best Practices

### Scenario 3.1: Database Workload

**User Prompt:**
> "I'm running PostgreSQL and want to optimize performance"

**WITHOUT MCP:**
```
For database optimization, you can create a performance profile.
You'll probably want:
- Some CPU isolation
- Maybe hugepages
- Not sure if RT kernel is needed

Check the PPC documentation for flags.
```

**WITH MCP:**
```
Great! I have a "Database Server" template optimized for PostgreSQL.

📋 Database Workload Analysis:
- Type: Database Server (PostgreSQL/MySQL/MongoDB/Oracle)
- RT Kernel: ❌ NOT needed (adds overhead without benefit)
- DPDK: ❌ NOT needed (databases use kernel networking)
- Focus: Memory optimization + consistent performance

🔧 Recommended Configuration:
✓ Hugepages: 2M (reduces TLB misses for large memory pools)
✓ CPU Isolation: Moderate (prevents noisy neighbors)
✓ NUMA Awareness: Enabled (improves memory locality)
✓ Power Mode: default (balanced performance/power)
✗ RT Kernel: Disabled (unnecessary overhead)
✗ Hyperthreading: Keep enabled (databases benefit from it)

💾 Memory Configuration:
For PostgreSQL with shared_buffers = 32GB:
- Hugepages (2M): ~16,384 pages (32GB)
- Reserved CPUs: 4-8 (for system)
- Isolated CPUs: Remaining (for PostgreSQL)

📊 Why NOT RT Kernel?
1. Databases are throughput-oriented, not latency-critical
2. RT kernel adds scheduling overhead
3. Context switching overhead outweighs benefits
4. Kernel I/O path is already optimized for databases

⚙️ PostgreSQL-Specific Tuning:
In addition to performance profile, tune PostgreSQL:
- huge_pages = on
- shared_buffers = 32GB (example)
- effective_cache_size = 96GB (example)
- work_mem = 256MB

Would you like me to generate the profile?
```

---

### Scenario 3.2: AI/ML Inference

**User Prompt:**
> "We're deploying TensorFlow serving for real-time inference"

**WITHOUT MCP:**
```
You can create a performance profile. Try isolating CPUs for your inference workload.
Maybe something like:

podman run ... performance-profile-creator \
  --reserved-cpu-count 4 \
  --isolated-cpu-count 12 \
  --must-gather-dir-path /must-gather
```

**WITH MCP:**
```
Perfect! I have an "AI/ML Inference" template for TensorFlow Serving.

🤖 AI Inference Workload Analysis:
- Type: AI/ML Inference (TensorFlow Serving, PyTorch, ONNX Runtime)
- Latency Goal: Predictable inference time (not ultra-low)
- RT Kernel: ❌ NOT needed (adds complexity without benefit)
- Focus: CPU isolation + consistent performance

🔧 Recommended Configuration:
✓ CPU Isolation: Aggressive (dedicated cores for inference)
✓ NUMA Awareness: Enabled (improves memory bandwidth)
✓ Power Mode: low-latency (consistent performance)
✓ Hyperthreading: Keep enabled (ML frameworks can use it)
✗ RT Kernel: Disabled (inference isn't hard real-time)
✗ DPDK: Disabled (HTTP/gRPC uses kernel networking)

🧮 CPU Allocation Strategy:

Option 1: Single Model Per Node
- Reserved: 4 CPUs (system/kubelet)
- Isolated: 44 CPUs (TensorFlow serving)
- Per-pod limits: 8-16 CPUs (adjust based on model)

Option 2: Multiple Models Per Node
- Reserved: 4 CPUs
- Isolated: 44 CPUs
- Multiple pods with guaranteed QoS
- Each pod gets dedicated isolated CPUs

📊 Why This Configuration?
1. ✓ Predictable latency (CPU isolation prevents interference)
2. ✓ High throughput (keep HT enabled)
3. ✓ Good for batched inference (multiple requests/batch)
4. ✓ Works with GPU/CPU inference

⚠️ Important Considerations:
- TensorFlow Serving CPU: Use MKL-optimized builds
- Set OMP_NUM_THREADS = pod CPU limit
- Use guaranteed QoS (resources.limits = requests)
- Consider topology manager for NUMA placement

📈 Performance Tips:
1. Batching: Enable dynamic batching in TF Serving
2. Threading: Set --tensorflow_inter_op_parallelism
3. Memory: Pre-allocate memory to avoid page faults
4. Monitoring: Watch for CPU throttling

Would you like me to generate the profile and example pod spec?
```

---

## Category 4: Error Prevention

### Scenario 4.1: Conflicting Requirements

**User Prompt:**
> "I want maximum throughput AND ultra-low latency AND lowest power consumption"

**WITHOUT MCP:**
```
Sure! Here's a configuration:

podman run ... performance-profile-creator \
  --reserved-cpu-count 4 \
  --isolated-cpu-count 40 \
  --rt-kernel true \
  --must-gather-dir-path /must-gather
```
*Doesn't explain the trade-offs*

**WITH MCP:**
```
⚠️ Conflicting Requirements Detected!

I see you want:
1. ✓ Maximum throughput
2. ✓ Ultra-low latency
3. ✓ Lowest power consumption

❌ Problem: These goals conflict with each other!

📊 Trade-off Analysis:

┌─────────────────────┬─────────────┬─────────┬───────────┐
│ Configuration       │ Throughput  │ Latency │ Power     │
├─────────────────────┼─────────────┼─────────┼───────────┤
│ Ultra-Low Latency   │ Medium ⚠️   │ Best ✓  │ High ❌   │
│ (C-states disabled) │             │         │           │
├─────────────────────┼─────────────┼─────────┼───────────┤
│ Maximum Throughput  │ Best ✓      │ Medium ⚠│ Medium 〰 │
│ (HT enabled)        │             │         │           │
├─────────────────────┼─────────────┼─────────┼───────────┤
│ Power Efficient     │ Lower ❌    │ Higher ❌│ Best ✓   │
│ (C-states enabled)  │             │         │           │
└─────────────────────┴─────────────┴─────────┴───────────┘

💡 Choose Your Priority:

**Option 1: Latency-First (Recommended for 5G/Trading)**
- Power Mode: ultra-low-latency (CPUs stay at max frequency)
- Hyperthreading: Disabled (deterministic latency)
- RT Kernel: Enabled
- Trade-off: 40-50% higher power, 30% lower throughput
- Use Case: 5G RAN, High-frequency trading

**Option 2: Throughput-First (Recommended for Databases/AI)**
- Power Mode: low-latency (balanced)
- Hyperthreading: Enabled (more logical CPUs)
- RT Kernel: Disabled
- Trade-off: Slightly higher latency variance
- Use Case: Databases, Batch processing, AI inference

**Option 3: Balanced (Recommended for Telco VNF)**
- Power Mode: low-latency
- Hyperthreading: Enabled
- RT Kernel: Enabled
- Trade-off: Moderate power consumption
- Use Case: Telco VNFs, Media processing

**Option 4: Power-First (NOT recommended for performance workloads)**
- Power Mode: default
- CPU frequency scaling: Enabled
- Trade-off: Unpredictable latency, lower throughput
- Use Case: Dev/test environments only

Which priority matches your actual requirement?
```

---

### Scenario 4.2: Missing Required Information

**User Prompt:**
> "Create me a performance profile"

**WITHOUT MCP:**
```
Here's a basic example:

podman run ... performance-profile-creator \
  --mcp-name worker-cnf \
  --reserved-cpu-count 4 \
  --rt-kernel true \
  --must-gather-dir-path /must-gather \
  > profile.yaml
```
*Provides generic command without understanding requirements*

**WITH MCP:**
```
I'd be happy to help! But I need to understand your requirements first.

❓ Let me ask you some questions:

**1. Workload Type** (most important)
What kind of application are you running?
- [ ] 5G/RAN (ultra-low latency telecom)
- [ ] Telco VNF (virtual network functions)
- [ ] Database (PostgreSQL, MySQL, MongoDB, etc.)
- [ ] AI/ML (inference or training)
- [ ] HPC (scientific computing)
- [ ] Financial trading
- [ ] Media processing (transcoding, streaming)
- [ ] Custom/Other (please describe)

**2. Latency Requirements**
- [ ] Ultra-low latency (<100 microseconds) - needs RT kernel
- [ ] Low latency (<10 milliseconds) - standard kernel OK
- [ ] Normal latency (best effort) - no special tuning

**3. Network Requirements**
- [ ] High-speed packet processing (needs DPDK)
- [ ] Standard networking (kernel stack OK)
- [ ] Don't know / Not applicable

**4. Must-Gather Data**
Do you have must-gather output?
- [ ] Yes (provide path: /path/to/must-gather)
- [ ] No (I'll generate a template profile)

**5. MachineConfigPool Name**
What's your MCP name? (e.g., "worker-cnf", "worker-perf")
- Required for profile generation

---

**Quick Start Options:**

If you know your workload, try these:
1. "I need a profile for 5G RAN workloads"
2. "I'm running PostgreSQL and want better performance"
3. "I need to optimize for TensorFlow inference"

Or provide your must-gather path and I'll analyze your hardware first!
```

---

## Category 5: Educational Explanations

### Scenario 5.1: Understanding RT Kernel

**User Prompt:**
> "Should I enable RT kernel?"

**WITHOUT MCP:**
```
RT kernel provides better latency. Enable it with --rt-kernel true.
```

**WITH MCP:**
```
Great question! Let me explain RT (Real-Time) kernel and help you decide.

🔍 What is RT Kernel?

RT kernel (PREEMPT_RT) provides deterministic, bounded latency by:
- Making kernel code preemptible (can be interrupted)
- Converting spinlocks to mutexes (reduces lock contention)
- Implementing priority inheritance (prevents priority inversion)
- Providing fine-grained control over scheduling

📊 RT Kernel Decision Tree:

                    Do you need ultra-low latency?
                              │
                 ┌────────────┴────────────┐
                 │                         │
               YES                        NO
                 │                         │
         ┌───────┴────────┐               │
         │                │               │
    < 100μs         < 10ms               │
      │                │                 │
   Use RT          Maybe RT          Don't use RT
      │                │                 │
      │                │                 │
   ┌──┴──┐         ┌──┴──┐          ┌──┴──┐
   │ 5G  │         │Telco│          │ DB  │
   │ RAN │         │ VNF │          │ AI  │
   └─────┘         └─────┘          └─────┘

✅ **Enable RT Kernel If:**
1. **5G RAN workloads**
   - Base station processing needs <100μs latency
   - Radio frame timing is critical
   - Missing deadlines = dropped calls

2. **High-frequency trading**
   - Microsecond latency = money
   - Predictable order execution time
   - Minimize worst-case latency

3. **Industrial control systems**
   - Safety-critical timing
   - Bounded response time required
   - Regulatory compliance

4. **Low-latency telco VNFs**
   - Session border controllers
   - Some packet processing workloads
   - Real-time media processing

❌ **DON'T Enable RT Kernel If:**
1. **Database workloads**
   - Throughput > latency
   - RT overhead reduces throughput by 5-15%
   - Normal kernel is already well-optimized

2. **AI/ML inference**
   - Inference time dominated by computation
   - Scheduler latency is negligible
   - RT provides no benefit

3. **Batch processing**
   - No real-time requirements
   - Throughput is king
   - RT is unnecessary overhead

4. **Web services / APIs**
   - Network latency >> kernel latency
   - Normal kernel is sufficient
   - Focus on application optimization

⚠️ **RT Kernel Trade-offs:**
- ✓ Pro: Bounded, predictable latency
- ✓ Pro: Better worst-case performance
- ❌ Con: ~5-15% throughput reduction
- ❌ Con: Increased complexity
- ❌ Con: Some kernel features unavailable
- ❌ Con: Requires RT-aware application design

📈 **Latency Comparison Example:**
┌─────────────────┬──────────┬──────────┬──────────┐
│ Kernel Type     │ Avg      │ P99      │ Max      │
├─────────────────┼──────────┼──────────┼──────────┤
│ Standard        │ 10μs     │ 500μs    │ 5ms      │
│ Standard + Tune │ 8μs      │ 100μs    │ 2ms      │
│ RT Kernel       │ 12μs     │ 50μs     │ 80μs     │
└─────────────────┴──────────┴──────────┴──────────┘

Notice: RT has higher average but MUCH better worst-case!

🎯 **Your Situation:**
Tell me about your workload, and I'll recommend whether RT kernel is appropriate!
```

---

### Scenario 5.2: Understanding CPU Isolation

**User Prompt:**
> "What's the difference between isolated and reserved CPUs?"

**WITHOUT MCP:**
```
- Isolated CPUs: For your workload
- Reserved CPUs: For system tasks

Use --isolated-cpu-count and --reserved-cpu-count flags.
```

**WITH MCP:**
```
Excellent question! This is one of the most important concepts for performance tuning.

🎯 CPU Types in Performance Profile:

┌──────────────────────────────────────────────────────────┐
│                   Total CPUs (e.g., 48)                   │
├─────────────────────┬────────────────────────────────────┤
│  RESERVED CPUs (4)  │      ISOLATED CPUs (44)            │
├─────────────────────┼────────────────────────────────────┤
│ • System tasks      │ • Your application workloads       │
│ • kubelet           │ • No system tasks scheduled here   │
│ • System daemons    │ • Exclusive use                    │
│ • Kernel threads    │ • No interruptions                 │
│ • Network stack     │ • Predictable performance          │
└─────────────────────┴────────────────────────────────────┘

📋 **Reserved CPUs (System CPUs)**

**Purpose:**
- Run all system-level tasks (kubelet, systemd, kernel workers)
- Handle interrupts (IRQs) from hardware devices
- Run infrastructure pods (CNI, CSI, logging, monitoring)
- Process kernel threads and background tasks

**Characteristics:**
- ✓ Shared among many processes
- ✓ Can be oversubscribed
- ✓ Handle all the "noisy neighbor" tasks
- ⚠️ Variable CPU utilization
- ⚠️ Unpredictable latency

**How Many to Reserve?**
- Minimum: 2 CPUs (very small nodes)
- Typical: 4 CPUs (most workloads)
- Recommended: 4-8 CPUs (busy clusters)
- Maximum: ~10% of total (high-throughput systems)

**Example: 48-CPU node**
- Light system load: 4 reserved CPUs (8%)
- Medium system load: 6 reserved CPUs (12%)
- Heavy system load: 8 reserved CPUs (17%)

---

📋 **Isolated CPUs (Application CPUs)**

**Purpose:**
- Dedicated exclusively to your application pods
- No system tasks scheduled (except your workload)
- No kernel threads (moved to reserved CPUs)
- No interrupts (IRQs moved to reserved CPUs)

**Characteristics:**
- ✓ Exclusive use by your application
- ✓ No interruptions from system tasks
- ✓ Predictable, consistent performance
- ✓ Lower latency variance
- ✗ Can't be oversubscribed (strict limits)

**How to Use:**
Your pods MUST request these CPUs:
```yaml
resources:
  requests:
    cpu: 4
    memory: 8Gi
  limits:
    cpu: 4  # Must match requests (guaranteed QoS)
    memory: 8Gi
```

**Example: 48-CPU node with 44 isolated CPUs**
- Pod 1: requests 16 CPUs → gets CPUs 4-19
- Pod 2: requests 8 CPUs → gets CPUs 20-27
- Pod 3: requests 8 CPUs → gets CPUs 28-35
- Remaining: 12 CPUs available for more pods

---

🔧 **Real-World Example:**

**Bad Configuration (NO isolation):**
```
All 48 CPUs shared:
CPU 0-47: Your app + kubelet + system + logging + monitoring + ...
Result: Latency spikes, unpredictable performance
```

**Good Configuration (WITH isolation):**
```
Reserved (CPU 0-3):
  - kubelet
  - systemd
  - logging
  - monitoring
  - all IRQs

Isolated (CPU 4-47):
  - ONLY your application pods
  - Nothing else scheduled here
  
Result: Predictable, low-latency performance
```

---

📊 **Impact Comparison:**

**Scenario: 5G RAN workload, 48-CPU node**

Without Isolation:
- P50 latency: 50μs ✓
- P99 latency: 5000μs ❌ (100x worse!)
- Max latency: 50ms ❌ (missed deadlines)

With Isolation (4 reserved, 44 isolated):
- P50 latency: 52μs ✓
- P99 latency: 80μs ✓ (62x better!)
- Max latency: 150μs ✓ (333x better!)

---

⚙️ **Advanced: CPU Affinities**

When you create a performance profile, it configures:

1. **kubelet** → Reserved CPUs only
2. **System daemons** → Reserved CPUs only
3. **IRQ handlers** → Reserved CPUs only
4. **RCU callbacks** → Reserved CPUs only
5. **Workqueue threads** → Reserved CPUs only

Your application pods → Isolated CPUs only (via cpuset)

---

🎯 **Sizing Guidelines:**

**Reserved CPU Calculation:**
```
Base: 2-4 CPUs (minimum for system)
+ 1 CPU per 100 pods (scale with pod count)
+ 1-2 CPUs if heavy logging/monitoring
+ 1-2 CPUs if high network throughput
= Total reserved CPUs
```

**Examples:**
- 48 CPUs, 20 pods, light logging: 4 reserved, 44 isolated
- 96 CPUs, 100 pods, heavy logging: 8 reserved, 88 isolated
- 128 CPUs, 200 pods, high network: 10 reserved, 118 isolated

---

💡 **Important Gotchas:**

1. **Pods must request guaranteed QoS:**
   ```yaml
   resources:
     requests:
       cpu: 4  # Must be integer
       memory: 8Gi
     limits:
       cpu: 4  # Must equal requests
       memory: 8Gi
   ```

2. **CPU requests must be integers:**
   ❌ cpu: 0.5 (won't use isolated CPUs)
   ✓ cpu: 1 (will use isolated CPUs)

3. **Can't oversubscribe isolated CPUs:**
   If you have 44 isolated CPUs, you can run:
   - ✓ 44 pods with 1 CPU each
   - ✓ 11 pods with 4 CPUs each
   - ❌ 45 pods with 1 CPU each (one will be pending)

4. **Watch your utilization:**
   Isolated CPUs sitting idle = wasted resources
   Consider mixing isolated + shared workloads

---

🎯 **Your Situation:**
Tell me about your workload and cluster size, and I'll calculate the optimal CPU allocation!

Questions:
1. How many CPUs per node?
2. How many pods per node?
3. What's your latency requirement?
4. What workload type?
```

---

## Category 6: Iterative Refinement

### Scenario 6.1: Starting from Scratch

**User Prompt (Iteration 1):**
> "I'm new to performance tuning, where do I start?"

**WITHOUT MCP:**
```
Start by collecting must-gather data, then run PPC tool.
```

**WITH MCP (Iteration 1):**
```
Welcome! I'll guide you through the process step by step.

🎯 Performance Tuning Workflow:

Step 1: Understand Your Workload (that's what we're doing now!)
Step 2: Collect Cluster Hardware Info (must-gather)
Step 3: Choose Configuration Template
Step 4: Validate Against Hardware
Step 5: Generate Performance Profile
Step 6: Apply and Test
Step 7: Monitor and Iterate

Let's start with Step 1:

❓ What kind of application are you running?
- Database (PostgreSQL, MySQL, etc.)
- Web service / API
- Data processing / Analytics
- Telecom / Networking
- AI / Machine Learning
- Something else?

Or describe it in your own words!
```

**User Response (Iteration 2):**
> "It's a gRPC service that processes requests, needs to be fast"

**WITH MCP (Iteration 2):**
```
Great! A gRPC service. Let me ask a few more questions:

1. **Latency requirement:**
   - What's your latency SLA? (e.g., P99 < 10ms)
   - Is it user-facing (needs consistently low latency)?

2. **Request pattern:**
   - Requests per second?
   - Request size (small/large)?
   - CPU-intensive processing or I/O-bound?

3. **Current issues:**
   - Are you experiencing latency spikes?
   - Inconsistent performance?
   - Throughput bottlenecks?

Based on your answers, I'll recommend:
- Whether you need CPU isolation
- If RT kernel would help
- Appropriate performance profile template
```

**User Response (Iteration 3):**
> "P99 should be under 5ms, user-facing, 10k RPS, CPU-intensive, seeing latency spikes up to 100ms"

**WITH MCP (Iteration 3):**
```
Perfect! Now I have enough information.

📊 Analysis:
- P99 target: <5ms ✓ Achievable without RT kernel
- User-facing: ✓ Needs consistent latency
- 10k RPS: ✓ Moderate throughput
- CPU-intensive: ✓ Benefits from CPU isolation
- Latency spikes (100ms): ⚠️ Likely "noisy neighbor" problem

🎯 Root Cause of Latency Spikes:
Your 100ms spikes are likely caused by:
1. System processes interrupting your app (kubelet, logging)
2. IRQ processing on same CPUs
3. CPU scheduling delays
4. Lack of guaranteed QoS

✅ Recommended Solution: "AI/ML Inference" template
(Yes, even though you're not doing AI - the profile fits your needs!)

🔧 Configuration:
- CPU Isolation: ✓ Enabled (eliminates noisy neighbors)
- RT Kernel: ✗ Disabled (5ms is achievable without it)
- Hyperthreading: ✓ Keep enabled (helps with throughput)
- Power Mode: low-latency (reduces CPU frequency transitions)
- Reserved CPUs: 4-6 (system tasks)
- Isolated CPUs: Remaining (your gRPC service)

📈 Expected Improvement:
- Before: P99 = 100ms (spikes)
- After: P99 < 5ms (consistent)
- Reduction: ~95% improvement in tail latency

🎯 Next Steps:
1. Provide must-gather path → I'll validate hardware
2. Generate performance profile
3. Update your pod spec (guaranteed QoS)
4. Apply and test
5. Monitor P99 latency

Ready to proceed? Share your must-gather path!
```

---

## Category 7: Multi-Step Workflows

### Scenario 7.1: Complete E2E Journey

**User Prompt:**
> "Help me create a profile for my 5G cluster"

**WITH MCP - Full Conversation:**

```
🤖 Assistant (Step 1 - Understanding):
Great! 5G workloads need special tuning. Let me understand your setup:

1. What type of 5G workload?
   - DU (Distributed Unit) - needs ultra-low latency
   - CU (Central Unit) - less stringent latency
   - Core (UPF, SMF, etc.) - throughput-focused
   - All of the above?

2. Do you have must-gather data?
   - If yes, provide path
   - If no, I'll guide you to collect it
```

```
👤 User:
It's DU workload, I have must-gather at /tmp/mg-cluster1
```

```
🤖 Assistant (Step 2 - Hardware Analysis):
Perfect! Let me analyze your hardware...

[Analyzing /tmp/mg-cluster1...]

✅ Hardware Analysis Complete:

📊 Cluster Topology:
- Nodes: 3 worker nodes
- CPUs per node: 56 (28 physical cores × 2 with HT)
- Architecture: x86_64 (Intel Xeon Gold 6348)
- NUMA nodes: 2 (28 CPUs each)
- Memory: 256GB per node
- NICs: Intel E810 (100Gbps, DPDK-capable) ✓

✅ DU Requirements Check:
- ✓ Sufficient CPUs (56 available)
- ✓ DPDK-capable NICs
- ✓ NUMA topology (good for isolation)
- ✓ Hyperthreading available (will disable for DU)
- ✓ 1G hugepages supported

⚠️ Important Finding:
Your CPUs support both 1G and 2M hugepages.
For DU workloads, 1G is recommended for fronthaul processing.

Ready to generate the profile?
```

```
👤 User:
Yes, generate it
```

```
🤖 Assistant (Step 3 - Profile Generation):
Generating 5G DU performance profile...

✅ Generated Configuration:

```yaml
apiVersion: performance.openshift.io/v2
kind: PerformanceProfile
metadata:
  name: 5g-du-profile
spec:
  cpu:
    isolated: "4-55"  # 52 CPUs for DU workload
    reserved: "0-3"   # 4 CPUs for system
  hugepages:
    defaultHugepagesSize: 1G
    pages:
    - size: 1G
      count: 16  # 16GB for DPDK
      node: 0
    - size: 1G
      count: 16
      node: 1
  realTimeKernel:
    enabled: true
  numa:
    topologyPolicy: single-numa-node
  nodeSelector:
    node-role.kubernetes.io/worker-cnf: ""
  machineConfigPoolSelector:
    pools.operator.machineconfiguration.openshift.io/worker-cnf: ""
  userLevelNetworking:
    enabled: true
  globallyDisableIrqLoadBalancing: true
  workloadHints:
    realTime: true
    highPowerConsumption: true
    perPodPowerManagement: false
```

📋 Configuration Explanation:

**CPU Allocation:**
- Reserved (0-3): System tasks, kubelet
- Isolated (4-55): Your DU workload (52 CPUs)
  - NUMA 0: CPUs 4-27 (24 CPUs)
  - NUMA 1: CPUs 28-55 (28 CPUs)

**Why Disable HT?**
❌ Hyperthreading disabled via workloadHints (realTime: true)
- DU needs deterministic timing
- HT causes L1/L2 cache thrashing
- Physical cores only = more predictable

**Hugepages:**
- 32GB total (16GB per NUMA node)
- 1G pages (better for DU than 2M)
- NUMA-local allocation

**RT Kernel:**
- ✓ Enabled (required for <100μs latency)
- Provides bounded scheduling latency
- PREEMPT_RT patches applied

**DPDK:**
- ✓ Enabled (userLevelNetworking: true)
- Required for fronthaul processing
- Poll-mode drivers (no interrupts)

**Power Mode:**
- highPowerConsumption: true
- Disables C-states (CPUs always awake)
- Maximum frequency always
- Trade-off: Higher power for lower latency

⚠️ Important Notes:
1. This configuration will REBOOT your nodes (MachineConfig change)
2. Ensure your DU pods use guaranteed QoS
3. Match pod CPU requests to multiples of NUMA CPUs
4. Test in dev environment first!

📝 Next Steps:

1. **Save the profile:**
   ```bash
   kubectl apply -f 5g-du-profile.yaml
   ```

2. **Wait for nodes to reboot:**
   ```bash
   watch kubectl get mcp
   # Wait for worker-cnf to show UPDATED=True
   ```

3. **Verify configuration:**
   ```bash
   # Check isolated CPUs
   oc debug node/worker-0
   cat /sys/devices/system/cpu/isolated
   # Should show: 4-55
   
   # Check hugepages
   cat /proc/meminfo | grep Huge
   # Should show 1G pages allocated
   ```

4. **Update DU pod spec:**
   ```yaml
   spec:
     containers:
     - name: du
       resources:
         requests:
           cpu: 24  # Integer, use full NUMA node
           memory: 32Gi
           hugepages-1Gi: 8Gi
         limits:
           cpu: 24  # Must match requests
           memory: 32Gi
           hugepages-1Gi: 8Gi
       securityContext:
         capabilities:
           add: ["IPC_LOCK", "SYS_RESOURCE"]
   ```

5. **Deploy and monitor:**
   - Apply your DU workload
   - Monitor latency metrics
   - Check for dropped frames
   - Verify CPU isolation: `oc adm top pod`

Would you like me to explain any part in more detail?
Or help with the pod spec?
```

**WITHOUT MCP - Same User Journey:**

```
User: Help me create a profile for my 5G cluster
